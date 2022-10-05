//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package credentialspec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/asm"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/s3"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/cihub/seelog"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	credentialsfetcherclient "github.com/aws/amazon-ecs-agent/agent/taskresource/grpcclient"
	"github.com/pkg/errors"
)

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
	taskARN                string
	region                 string
	executionCredentialsID string
	credentialsManager     credentials.Manager
	createdAt              time.Time
	desiredStatusUnsafe    resourcestatus.ResourceStatus
	knownStatusUnsafe      resourcestatus.ResourceStatus
	// appliedStatus is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatus                      resourcestatus.ResourceStatus
	resourceStatusToTransitionFunction map[resourcestatus.ResourceStatus]func() error
	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisioning failed.
	terminalReason     string
	terminalReasonOnce sync.Once
	// ssmClientCreator is a factory interface that creates new SSM clients. This is
	// needed mostly for testing.
	ssmClientCreator ssmfactory.SSMClientCreator
	// s3ClientCreator is a factory interface that creates new S3 clients. This is
	// needed mostly for testing.
	s3ClientCreator s3factory.S3ClientCreator
	// asmClientCreator is a factory interface that creates new secrets manager clients. This is
	// needed mostly for testing.
	asmClientCreator asmfactory.ClientCreator
	//	This stores credspec  arn and the corresponding service account name, domain name
	// * key := credentialspec:ssmARN, value := Path to kerberos tickets on the host machine
	// * key := credentialspec:asmARN, value := Path to kerberos tickets on the host machine
	ServiceAccountInfoMap map[string]ServiceAccountInfo
	// This stores the identifier associated with the kerberos tickets created for the task
	leaseid string
	// map to transform credentialspec values, key is a input credentialspec
	// Examples:
	// * key := credentialspec:file://credentialspec.json, value := credentialspec=file://credentialspec.json
	// * key := credentialspec:s3ARN, value := credentialspec=file://CredentialSpecResourceLocation/s3_taskARN_fileName.json
	// * key := credentialspec:ssmARN, value := credentialspec=file://CredentialSpecResourceLocation/ssm_taskARN_param.json
	CredSpecMap map[string]string
	// The essential map of credentialspecs needed for the containers. It stores the map with the credentialSpecARN as
	// the key container name as the value.
	// Example item := arn:aws:ssm:us-east-1:XXXXXXXXXXXXX:parameter/x/y/c:container-sql
	// This stores the map of a credential spec to corresponding container name
	credentialSpecContainerMap map[string]string
	//	This stores credspec contents associated to all the containers of the task
	credentialsFetcherRequest []string
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// ServiceAccountInfo contains account info associated to a credentialspec
type ServiceAccountInfo struct {
	serviceAccountName string
	domainName         string
}

// CredentialSpec object schema
type CredentialSpec struct {
	CmsPlugins       []string `json:"CmsPlugins"`
	DomainJoinConfig struct {
		Sid                string `json:"Sid"`
		MachineAccountName string `json:"MachineAccountName"`
		GUID               string `json:"Guid"`
		DNSTreeName        string `json:"DnsTreeName"`
		DNSName            string `json:"DnsName"`
		NetBiosName        string `json:"NetBiosName"`
	} `json:"DomainJoinConfig"`
	ActiveDirectoryConfig struct {
		GroupManagedServiceAccounts []struct {
			Name  string `json:"Name"`
			Scope string `json:"Scope"`
		} `json:"GroupManagedServiceAccounts"`
	} `json:"ActiveDirectoryConfig"`
}

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	s3ClientCreator s3factory.S3ClientCreator,
	credentialSpecContainerMap map[string]string) (*CredentialSpecResource, error) {
	s := &CredentialSpecResource{
		taskARN:                    taskARN,
		region:                     region,
		credentialsManager:         credentialsManager,
		executionCredentialsID:     executionCredentialsID,
		ssmClientCreator:           ssmClientCreator,
		asmClientCreator:           asmClientCreator,
		s3ClientCreator:            s3ClientCreator,
		CredSpecMap:                make(map[string]string),
		credentialSpecContainerMap: credentialSpecContainerMap,
		ServiceAccountInfoMap:      make(map[string]ServiceAccountInfo),
	}

	s.initStatusToTransition()
	return s, nil
}

// Create is used to retrieve credentialspec resources for a given task
func (cs *CredentialSpecResource) Create() error {
	var iamCredentials credentials.IAMRoleCredentials

	executionCredentials, ok := cs.credentialsManager.GetTaskCredentials(cs.getExecutionCredentialsID())
	if ok {
		iamCredentials = executionCredentials.GetIAMRoleCredentials()
	}

	var wg sync.WaitGroup
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	for credSpecStr := range cs.credentialSpecContainerMap {
		credSpecSplit := strings.SplitAfterN(credSpecStr, "credentialspec:", 2)
		if len(credSpecSplit) != 2 {
			seelog.Errorf("Invalid credentialspec: %s", credSpecStr)
			continue
		}
		credSpecValue := credSpecSplit[1]

		parsedARN, err := arn.Parse(credSpecValue)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
		parsedARNService := parsedARN.Service
		if parsedARNService == "s3" {
			wg.Add(1)
			cs.handleS3CredentialspecFile(credSpecStr, credSpecValue, iamCredentials, &wg, errorEvents)
		} else if parsedARNService == "secretsmanager" {
			wg.Add(1)
			cs.handleASMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials, &wg, errorEvents)
		} else if parsedARNService == "ssm" {
			wg.Add(1)
			go cs.handleSSMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials, &wg, errorEvents)
		} else {
			err = errors.New("unsupported credentialspec ARN, only secretsmanager/ssm ARNs are valid")
			cs.setTerminalReason(err.Error())
			return err
		}
	}
	wg.Wait()
	close(errorEvents)
	if len(errorEvents) > 0 {
		var terminalReasons []string
		for err := range errorEvents {
			terminalReasons = append(terminalReasons, err.Error())
		}

		errorString := strings.Join(terminalReasons, ";")
		cs.setTerminalReason(errorString)
		return errors.New(errorString)
	}

	seelog.Infof("credentials fetcher daemon request: %v", cs.credentialsFetcherRequest)
	// Create kerberos tickets for the gMSA service accounts on the host location /var/credentials-fetcher/krbdir
	if len(cs.credentialsFetcherRequest) > 0 {
		//set up server connection to communicate with credentials fetcher daemon
		conn, err := credentialsfetcherclient.GetGrpcClientConnection()
		seelog.Infof("grpc connection: %v", conn)
		if err != nil {
			seelog.Debugf("failed to connect with credentials fetcher daemon: %s", err)
			return err
		}
		// make the grpc call to add kerberos lease api to create kerberos tickets for the gmsa account
		response, err := credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).AddKerberosLease(context.Background(), cs.credentialsFetcherRequest)

		if err != nil {
			seelog.Debugf("failed to create kerberos tickets associated service account, error: %s", err)
			cs.setTerminalReason(err.Error())
			return err
		}

		cs.leaseid = response.LeaseId
		seelog.Infof("credentials fetcher response leaseid: %v", cs.leaseid)

		//update the mapping of credspec ARN to the kerberos ticket location on the container instance
		for _, kerberosTicketLocation := range response.KerberosTicketPaths {
			for k, v := range cs.ServiceAccountInfoMap {
				result := strings.Contains(strings.ToLower(kerberosTicketLocation), strings.ToLower(v.serviceAccountName))
				if result {
					cs.CredSpecMap[k] = kerberosTicketLocation
					break
				}
			}
		}
	}

	return nil
}

func (cs *CredentialSpecResource) handleS3CredentialspecFile(originalCredentialspec, credentialspecS3ARN string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	_, err := arn.Parse(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	bucket, key, err := s3.ParseS3ARN(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	s3Client := cs.s3ClientCreator.NewS3Client(cs.region, iamCredentials)

	credSpecJsonStringUnformatted, err := s3.GetObject(bucket, key, s3Client)

	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	credSpecJsonStringBytes := &bytes.Buffer{}
	json.Compact(credSpecJsonStringBytes, []byte(credSpecJsonStringUnformatted))
	credSpecJsonString := credSpecJsonStringBytes.String()

	cs.updateCredSpecMapping(originalCredentialspec, credSpecJsonString)
}

func (cs *CredentialSpecResource) handleASMCredentialspecFile(originalCredentialspec, credentialspecASMARN string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	parsedARN, err := arn.Parse(credentialspecASMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	// An ASM ARN is in the form of arn:aws:secretsmanager:us-west-2:secret:test-8mJ3EJ. The parsed ARN value
	// would be secret: test-8mJ3EJ. The following code gets the ASM secret by passing "test" value to the
	// GetSecretFromASM method to retrieve the value in the secrets store.
	asmArray := strings.SplitN(parsedARN.Resource, "secret:", 2)
	if len(asmArray) != 2 {
		err := fmt.Errorf("the provided ASM arn:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
	asmKey := strings.SplitN(asmArray[1], "-", 2)
	if len(asmKey) != 2 {
		err := fmt.Errorf("the provided ASM key:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
	key := asmKey[0]
	asmClient := cs.asmClientCreator.NewASMClient(cs.region, iamCredentials)
	credSpecJsonString, err := asm.GetSecretFromASM(key, asmClient)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	cs.updateCredSpecMapping(originalCredentialspec, credSpecJsonString)
}

func (cs *CredentialSpecResource) handleSSMCredentialspecFile(originalCredentialspec, credentialspecSSMARN string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	parsedARN, err := arn.Parse(credentialspecSSMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	// An SSM ARN is in the form of arn:aws:ssm:us-west-2:123456789012:parameter/a/b. The parsed ARN value
	// would be parameter/a/b. The following code gets the SSM parameter by passing "/a/b" value to the
	// GetParametersFromSSM method to retrieve the value in the parameter.
	ssmParam := strings.SplitAfterN(parsedARN.Resource, "parameter", 2)
	if len(ssmParam) != 2 {
		err := fmt.Errorf("the provided SSM parameter:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
	ssmParams := []string{ssmParam[1]}

	ssmClient := cs.ssmClientCreator.NewSSMClient(cs.region, iamCredentials)
	seelog.Debugf("ssm secret resource: retrieving resource for secrets %v in region [%s] in task: [%s]", cs.region, ssmParams)
	ssmParamMap, err := ssm.GetSecretsFromSSM(ssmParams, ssmClient)
	if err != nil {
		errorEvents <- fmt.Errorf("fetching secret data from SSM Parameter Store in %s: %v", ssmParamMap, err)
		return
	}

	ssmParamData := ssmParamMap[ssmParam[1]]
	cs.updateCredSpecMapping(originalCredentialspec, ssmParamData)
}

func (cs *CredentialSpecResource) updateCredSpecMapping(credSpecInput, credSpecContent string) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	//parse json to extract the service account name and the domain name
	var credentialSpec CredentialSpec

	// Unmarshal or Decode the JSON to the interface.
	err := json.Unmarshal([]byte(credSpecContent), &credentialSpec)
	seelog.Infof("%v", credentialSpec)

	if err != nil {
		seelog.Debugf("Error unmarshalling credentialspec data %s", credSpecContent)
		return
	}

	serviceAccountName := credentialSpec.DomainJoinConfig.MachineAccountName
	domainName := credentialSpec.DomainJoinConfig.DNSName

	if len(serviceAccountName) != 0 && len(domainName) != 0 {
		cs.ServiceAccountInfoMap[credSpecInput] = ServiceAccountInfo{
			serviceAccountName: serviceAccountName,
			domainName:         domainName,
		}

		//build request array for credentials fetcher daemon
		cs.credentialsFetcherRequest = append(cs.credentialsFetcherRequest, credSpecContent)
	}
}

// Cleanup removes the credentialspec created for the task
func (cs *CredentialSpecResource) Cleanup() error {
	cs.clearKerberosTickets()
	return nil
}

// clearKerberosTickets cycles through the lease directory in the host machine
// and removes the associated kerberos tickets
func (cs *CredentialSpecResource) clearKerberosTickets() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if cs.leaseid != "" {
		//set up server connection to communicate with credentials fetcher daemon
		conn, err := credentialsfetcherclient.GetGrpcClientConnection()
		if err != nil {
			seelog.Debugf("failed to connect with credentials fetcher daemon: %s", err)
		}
		_, err = credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).DeleteKerberosLease(context.Background(), cs.leaseid)
		if err != nil {
			seelog.Debugf("Unable to cleanup kerberos tickets associated with leaseid: %s, error: %s", cs.leaseid, err)
		}
	}

	for key := range cs.CredSpecMap {
		if len(key) > 0 {
			delete(cs.CredSpecMap, key)
			delete(cs.ServiceAccountInfoMap, key)
		}
	}
}
