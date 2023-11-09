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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/asm"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/s3"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/cihub/seelog"

	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	credentialsfetcherclient "github.com/aws/amazon-ecs-agent/ecs-agent/gmsacredclient"
	"github.com/pkg/errors"
)

const (
	// envSkipCredentialsFetcherInvocation is an environment setting that can be used to skip
	// credentials fetcher daemon invocation. This is useful for integration and
	// functional-tests but should not be set for any non-test use-case.
	envSkipCredentialsFetcherInvocation = "ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION"
)

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
	*CredentialSpecResourceCommon
	// This stores the identifier associated with the kerberos tickets created for the task
	leaseID string
	// identify domainless or domain-joined gMSA
	isDomainlessGmsa bool
	//	This stores credspec  arn and the corresponding service account name, domain name
	// * key := credentialspec:ssmARN, value := corresponding ServiceAccountInfo
	// * key := credentialspec:asmARN, value := corresponding ServiceAccountInfo
	ServiceAccountInfoMap map[string]ServiceAccountInfo
	//	This stores credspec contents associated to all the containers of the task
	credentialsFetcherRequest []string
}

// ServiceAccountInfo contains account info associated to a credentialspec
type ServiceAccountInfo struct {
	serviceAccountName    string
	domainName            string
	domainlessGmsaUserArn string
	credentialSpecContent string
}

// DomainlessUserCredentials represents user credentials for accessing the gMSA principals
type DomainlessUserCredentials struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DomainName string `json:"domainName"`
}

// CredentialSpec object schema
type CredentialSpecSchema struct {
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

// CredentialSpec domainless object schema
type CredentialSpecDomainlessSchema struct {
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
		HostAccountConfig struct {
			PortableCcgVersion string `json:"PortableCcgVersion"`
			PluginGUID         string `json:"PluginGUID"`
			PluginInput        struct {
				CredentialArn string `json:"CredentialArn"`
			} `json:"PluginInput"`
		} `json:"HostAccountConfig"`
	} `json:"ActiveDirectoryConfig"`
}

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	s3ClientCreator s3factory.S3ClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	credentialSpecContainerMap map[string]string) (*CredentialSpecResource, error) {
	s := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			taskARN:                     taskARN,
			region:                      region,
			credentialsManager:          credentialsManager,
			executionCredentialsID:      executionCredentialsID,
			ssmClientCreator:            ssmClientCreator,
			s3ClientCreator:             s3ClientCreator,
			secretsmanagerClientCreator: asmClientCreator,
			CredSpecMap:                 make(map[string]string),
			credentialSpecContainerMap:  credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: make(map[string]ServiceAccountInfo),
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

	err := cs.retrieveCredentialSpecs(iamCredentials)
	if err != nil {
		return err
	}

	seelog.Infof("credentials fetcher daemon request: %v", cs.credentialsFetcherRequest)

	// Check if skip credential fetcher invocation check override is present
	skipSkipCredentialsFetcherInvocationCheck := utils.ParseBool(os.Getenv(envSkipCredentialsFetcherInvocation), false)
	if skipSkipCredentialsFetcherInvocationCheck {
		seelog.Info("Skipping credential fetcher invocation based on environment override")
		testKrbFilePath := "/tmp/tgt"
		os.Create(testKrbFilePath)
		// assign temporary variable for test
		cs.leaseID = "12345"
		for k := range cs.ServiceAccountInfoMap {
			cs.CredSpecMap[k] = testKrbFilePath
		}
		return nil
	}

	if cs.isDomainlessGmsa {
		err := cs.handleDomainlessKerberosTicketCreation()
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
	} else {
		err := cs.handleKerberosTicketCreation()
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
	}
	return nil
}

func (cs *CredentialSpecResource) retrieveCredentialSpecs(iamCredentials credentials.IAMRoleCredentials) error {
	cs.isDomainlessGmsa = false
	var wg sync.WaitGroup
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	for credSpecStr := range cs.credentialSpecContainerMap {
		cs.isDomainlessGmsa = strings.Contains(credSpecStr, "credentialspecdomainless")
		var credSpecSplit []string
		if cs.isDomainlessGmsa {
			credSpecSplit = strings.SplitAfterN(credSpecStr, "credentialspecdomainless:", 2)
		} else {
			credSpecSplit = strings.SplitAfterN(credSpecStr, "credentialspec:", 2)
		}

		if len(credSpecSplit) != 2 {
			seelog.Errorf("Invalid credentialspec: %s", credSpecStr)
			continue
		}

		credSpecValue := credSpecSplit[1]
		if strings.HasPrefix(credSpecValue, "file://") {
			wg.Add(1)
			go cs.handleCredentialspecFile(credSpecStr, credSpecValue, &wg, errorEvents)
			continue
		}

		parsedARN, err := arn.Parse(credSpecValue)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
		parsedARNService := parsedARN.Service
		switch parsedARNService {
		case "s3":
			wg.Add(1)
			go cs.handleS3CredentialspecFile(credSpecStr, credSpecValue, iamCredentials, &wg, errorEvents)
		case "ssm":
			wg.Add(1)
			go cs.handleSSMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials, &wg, errorEvents)
		default:
			err = errors.New("unsupported credentialspec ARN, only s3/ssm ARNs are valid")
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
	return nil
}

func (cs *CredentialSpecResource) handleDomainlessKerberosTicketCreation() error {
	// Create kerberos tickets for the gMSA service accounts in domain-less mode on the host location /var/credentials-fetcher/krbdir
	var iamCredentials credentials.IAMRoleCredentials
	for k, v := range cs.ServiceAccountInfoMap {
		if v.domainlessGmsaUserArn != "" {
			// get domain-user credentials from secrets manager
			executionCredentials, ok := cs.credentialsManager.GetTaskCredentials(cs.getExecutionCredentialsID())
			if ok {
				iamCredentials = executionCredentials.GetIAMRoleCredentials()
			}

			asmClient := cs.secretsmanagerClientCreator.NewASMClient(cs.region, iamCredentials)

			asmSecretData, err := asm.GetSecretFromASM(v.domainlessGmsaUserArn, asmClient)
			if err != nil {
				return fmt.Errorf("failed to retrieve credentials for domainless gMSA user %s: %w", v.domainlessGmsaUserArn, err)
			}
			creds := DomainlessUserCredentials{}
			if err := json.Unmarshal([]byte(asmSecretData), &creds); err != nil {
				return fmt.Errorf("failed to parse asmSecretData for the gMSA AD user: %w", err)
			}
			//set up server connection to communicate with credentials fetcher daemon
			conn, err := credentialsfetcherclient.GetGrpcClientConnection()
			if err != nil {
				seelog.Errorf("failed to connect with credentials fetcher daemon: %s", err)
				return err
			}
			seelog.Infof("grpc connection: %v", conn)

			response, err := credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).AddNonDomainJoinedKerberosLease(context.Background(),
				[]string{v.credentialSpecContent}, creds.Username, creds.Password, creds.DomainName)

			if err != nil {
				cs.setTerminalReason(err.Error())
				return fmt.Errorf("failed to create kerberos tickets associated service account %s: %w", v.domainlessGmsaUserArn, err)
			}
			seelog.Infof("credentials fetcher response leaseID: %v", cs.leaseID)
			cs.CredSpecMap[k] = response.KerberosTicketPaths[0]
		}
	}
	return nil
}
func (cs *CredentialSpecResource) HandleDomainlessKerberosTicketRenewal(iamCredentials credentials.IAMRoleCredentials) error {
	//update the region if it is not already set
	err := cs.UpdateRegionFromTask()
	if err != nil {
		return err
	}

	err = cs.retrieveCredentialSpecs(iamCredentials)
	if err != nil {
		return err
	}

	visitedDomainlessUser := make(map[string]bool)
	// Renew kerberos tickets for the gMSA service accounts in domain-less mode on the host location /var/credentials-fetcher/krbdir
	for _, v := range cs.ServiceAccountInfoMap {
		if v.domainlessGmsaUserArn != "" {
			_, ok := visitedDomainlessUser[v.domainlessGmsaUserArn]
			if !ok {
				visitedDomainlessUser[v.domainlessGmsaUserArn] = true

				// get domain-user credentials from secrets manager
				asmClient := cs.secretsmanagerClientCreator.NewASMClient(cs.region, iamCredentials)

				asmSecretData, err := asm.GetSecretFromASM(v.domainlessGmsaUserArn, asmClient)
				if err != nil {
					return fmt.Errorf("failed to retrieve credentials for domainless gMSA user %s: %w", v.domainlessGmsaUserArn, err)
				}
				creds := DomainlessUserCredentials{}
				if err := json.Unmarshal([]byte(asmSecretData), &creds); err != nil {
					return fmt.Errorf("failed to parse asmSecretData for the gMSA AD user: %w", err)
				}
				//set up server connection to communicate with credentials fetcher daemon
				conn, err := credentialsfetcherclient.GetGrpcClientConnection()
				if err != nil {
					seelog.Errorf("failed to connect with credentials fetcher daemon: %s", err)
					return err
				}
				seelog.Infof("grpc connection: %v", conn)

				_, err = credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).RenewNonDomainJoinedKerberosLease(context.Background(),
					creds.Username, creds.Password, creds.DomainName)

				if err != nil {
					cs.setTerminalReason(err.Error())
					return fmt.Errorf("failed to renew kerberos tickets associated service account %s: %w", v.domainlessGmsaUserArn, err)
				}
				seelog.Infof("renewal is successful: %v", cs.leaseID)
			}
		}
	}
	return nil
}

func (cs *CredentialSpecResource) handleKerberosTicketCreation() error {
	// Create kerberos tickets for the gMSA service accounts on the host location /var/credentials-fetcher/krbdir
	if len(cs.credentialsFetcherRequest) > 0 {
		//set up server connection to communicate with credentials fetcher daemon
		conn, err := credentialsfetcherclient.GetGrpcClientConnection()
		seelog.Infof("grpc connection: %v", conn)
		if err != nil {
			seelog.Errorf("failed to connect with credentials fetcher daemon: %s", err)
			return err
		}
		// make the grpc call to add kerberos lease api to create kerberos tickets for the gmsa account
		response, err := credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).AddKerberosLease(context.Background(), cs.credentialsFetcherRequest)

		if err != nil {
			seelog.Errorf("failed to create kerberos tickets associated service account, error: %s", err)
			cs.setTerminalReason(err.Error())
			return err
		}

		cs.leaseID = response.LeaseID
		seelog.Infof("credentials fetcher response leaseID: %v", cs.leaseID)

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

func (cs *CredentialSpecResource) handleCredentialspecFile(originalCredentialSpecFile, credentialSpec string, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	if !strings.HasPrefix(credentialSpec, "file://") {
		err := errors.New("invalid credentialspec file specification")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	fileName := strings.SplitAfterN(credentialSpec, "file://", 2)
	data, err := os.ReadFile(fileName[1])
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	credSpecData := string(data)

	err = cs.updateCredSpecMapping(originalCredentialSpecFile, credSpecData)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
}

func (cs *CredentialSpecResource) handleS3CredentialspecFile(originalCredentialSpecARN, credentialSpecS3ARN string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
	_, err := arn.Parse(credentialSpecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	bucket, key, err := s3.ParseS3ARN(credentialSpecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	s3Client, err := cs.s3ClientCreator.NewS3Client(bucket, cs.region, iamCredentials)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	credSpecJsonStringUnformatted, err := s3.GetObject(bucket, key, s3Client)

	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	credSpecJsonStringBytes := &bytes.Buffer{}
	json.Compact(credSpecJsonStringBytes, []byte(credSpecJsonStringUnformatted))
	credSpecJsonString := credSpecJsonStringBytes.String()

	err = cs.updateCredSpecMapping(originalCredentialSpecARN, credSpecJsonString)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
}

func (cs *CredentialSpecResource) handleSSMCredentialspecFile(originalCredentialSpecARN, credentialSpecSSMARN string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}

	parsedARN, err := arn.Parse(credentialSpecSSMARN)
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
	seelog.Errorf("ssm secret resource: retrieving resource for secrets %v in region [%s] in task: [%s]", cs.region, ssmParams)
	ssmParamMap, err := ssm.GetSecretsFromSSM(ssmParams, ssmClient)
	if err != nil {
		errorEvents <- fmt.Errorf("fetching secret data from SSM Parameter Store in %s: %v", ssmParamMap, err)
		return
	}

	ssmParamData := ssmParamMap[ssmParam[1]]
	err = cs.updateCredSpecMapping(originalCredentialSpecARN, ssmParamData)
	if err != nil {
		cs.setTerminalReason(err.Error())
		errorEvents <- err
		return
	}
}

// updateCredSpecMapping updates the mapping of credentialSpec input and the corresponding service account info(serviceAccountName, DomainNAme)
func (cs *CredentialSpecResource) updateCredSpecMapping(credSpecInput, credSpecContent string) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	var serviceAccountName string
	var domainName string
	var domainlessGmsaUserArn string
	//parse json to extract the service account name and the domain name
	if cs.isDomainlessGmsa {
		var credentialSpecDomainlessSchema CredentialSpecDomainlessSchema
		// Unmarshal or Decode the JSON to the interface for domainless gmsa.
		err := json.Unmarshal([]byte(credSpecContent), &credentialSpecDomainlessSchema)

		if err != nil {
			return fmt.Errorf("error unmarshalling credentialspec domainless %s : %w", credSpecContent, err)
		}

		serviceAccountName = credentialSpecDomainlessSchema.DomainJoinConfig.MachineAccountName
		domainName = credentialSpecDomainlessSchema.DomainJoinConfig.DNSName
		pluginInput := credentialSpecDomainlessSchema.ActiveDirectoryConfig.HostAccountConfig.PluginInput
		domainlessGmsaUserArn = pluginInput.CredentialArn

	} else {
		var credentialSpecSchema CredentialSpecSchema
		// Unmarshal or Decode the JSON to the interface for domainjoined gmsa.
		err := json.Unmarshal([]byte(credSpecContent), &credentialSpecSchema)

		if err != nil {
			return fmt.Errorf("Error unmarshalling credentialspec domain-joined %s : %w", credSpecContent, err)
		}

		serviceAccountName = credentialSpecSchema.DomainJoinConfig.MachineAccountName
		domainName = credentialSpecSchema.DomainJoinConfig.DNSName
		domainlessGmsaUserArn = ""
		//build request array for credentials fetcher daemon
		cs.credentialsFetcherRequest = append(cs.credentialsFetcherRequest, credSpecContent)
	}

	if len(serviceAccountName) > 0 && len(domainName) > 0 {
		cs.ServiceAccountInfoMap[credSpecInput] = ServiceAccountInfo{
			serviceAccountName:    serviceAccountName,
			domainName:            domainName,
			domainlessGmsaUserArn: domainlessGmsaUserArn,
			credentialSpecContent: credSpecContent,
		}
	}
	return nil
}

// update region if is not set
func (cs *CredentialSpecResource) UpdateRegionFromTask() error {
	// Parse taskARN
	parsedARN, err := arn.Parse(cs.taskARN)
	if err != nil {
		return err
	}

	cs.region = parsedARN.Region
	return nil
}

// Cleanup removes the credentialSpec created for the task
func (cs *CredentialSpecResource) Cleanup() error {
	cs.clearKerberosTickets()
	return nil
}

// clearKerberosTickets cycles through the lease directory in the host machine
// and removes the associated kerberos tickets
func (cs *CredentialSpecResource) clearKerberosTickets() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if cs.leaseID != "" {
		//set up server connection to communicate with credentials fetcher daemon
		conn, err := credentialsfetcherclient.GetGrpcClientConnection()
		if err != nil {
			seelog.Errorf("failed to connect with credentials fetcher daemon: %s", err)
		}
		_, err = credentialsfetcherclient.NewCredentialsFetcherClient(conn, time.Minute).DeleteKerberosLease(context.Background(), cs.leaseID)
		if err != nil {
			seelog.Errorf("Unable to cleanup kerberos tickets associated with leaseid: %s, error: %s", cs.leaseID, err)
		}
	}

	for key := range cs.CredSpecMap {
		if len(key) > 0 {
			delete(cs.CredSpecMap, key)
			delete(cs.ServiceAccountInfoMap, key)
		}
	}
}

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSON struct {
	*CredentialSpecResourceJSONCommon
	LeaseID string `json:"leaseID"`
}

func (cs *CredentialSpecResource) MarshallPlatformSpecificFields(credentialSpecResourceJSON *CredentialSpecResourceJSON) {
	credentialSpecResourceJSON.LeaseID = cs.leaseID
}

func (cs *CredentialSpecResource) UnmarshallPlatformSpecificFields(credentialSpecResourceJSON CredentialSpecResourceJSON) {
	cs.leaseID = credentialSpecResourceJSON.LeaseID
}
