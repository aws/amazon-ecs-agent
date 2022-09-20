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
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/cihub/seelog"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	credentialsfetcherclient "github.com/aws/amazon-ecs-agent/agent/taskresource/grpcclient"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
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
	// asmClientCreator is a factory interface that creates new secrets manager clients. This is
	// needed mostly for testing.
	asmClientCreator asmfactory.ClientCreator
	// This stores the identifier associated with the kerberos tickets created for the task
	leaseid string
	// map of path to kerberos tickets, key is an input credentialspec
	// Examples:
	// * key := credentialspec:asmARN, value := Path to kerberos tickets on the host machine
	// * key := credentialspec:ssmARN, value := Path to kerberos tickets on the host machine
	CredSpecMap map[string]string
	// The essential map of credentialspecs needed for the containers. It stores the map with the credentialSpecARN as
	// the key container name as the value.
	// Example item := arn:aws:ssm:us-east-1:XXXXXXXXXXXXX:parameter/x:container-sql
	// This stores the map of a credential spec to corresponding container name
	credentialSpecContainerMap map[string]string
	//	This stores credspec  arn and the corresponding service account name, domain name
	// * key := credentialspec:ssmARN, value := Path to kerberos tickets on the host machine
	// * key := credentialspec:asmARN, value := Path to kerberos tickets on the host machine
	ServiceAccountInfoMap map[string]ServiceAccountInfo
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

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	credentialSpecContainerMap map[string]string) (*CredentialSpecResource, error) {
	s := &CredentialSpecResource{
		taskARN:                    taskARN,
		region:                     region,
		credentialsManager:         credentialsManager,
		executionCredentialsID:     executionCredentialsID,
		ssmClientCreator:           ssmClientCreator,
		asmClientCreator:           asmClientCreator,
		CredSpecMap:                make(map[string]string),
		ServiceAccountInfoMap:      make(map[string]ServiceAccountInfo),
		credentialSpecContainerMap: credentialSpecContainerMap,
	}

	s.initStatusToTransition()
	return s, nil
}

func (cs *CredentialSpecResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(CredentialSpecCreated): cs.Create,
	}
	cs.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (cs *CredentialSpecResource) Initialize(resourceFields *taskresource.ResourceFields,
	_ status.TaskStatus,
	_ status.TaskStatus) {

	cs.credentialsManager = resourceFields.CredentialsManager
	cs.ssmClientCreator = resourceFields.SSMClientCreator
	cs.asmClientCreator = resourceFields.ASMClientCreator
	cs.initStatusToTransition()
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (cs *CredentialSpecResource) GetTerminalReason() string {
	return cs.terminalReason
}

func (cs *CredentialSpecResource) setTerminalReason(reason string) {
	cs.terminalReasonOnce.Do(func() {
		seelog.Debugf("credentialspec resource: setting terminal reason for credentialspec resource in task: [%s]", cs.taskARN)
		cs.terminalReason = reason
	})
}

// GetDesiredStatus safely returns the desired status of the task
func (cs *CredentialSpecResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.desiredStatusUnsafe
}

// SetDesiredStatus safely sets the desired status of the resource
func (cs *CredentialSpecResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.desiredStatusUnsafe = status
}

// DesiredTerminal returns true if the credentialspec's desired status is REMOVED
func (cs *CredentialSpecResource) DesiredTerminal() bool {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.desiredStatusUnsafe == resourcestatus.ResourceStatus(CredentialSpecRemoved)
}

// KnownCreated returns true if the credentialspec's known status is CREATED
func (cs *CredentialSpecResource) KnownCreated() bool {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.knownStatusUnsafe == resourcestatus.ResourceStatus(CredentialSpecCreated)
}

// TerminalStatus returns the last transition state of credentialspec
func (cs *CredentialSpecResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(CredentialSpecRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (cs *CredentialSpecResource) NextKnownState() resourcestatus.ResourceStatus {
	return cs.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (cs *CredentialSpecResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := cs.resourceStatusToTransitionFunction[nextState]
	if !ok {
		err := errors.Errorf("resource [%s]: transition to %s impossible", cs.GetName(),
			cs.StatusString(nextState))
		cs.setTerminalReason(err.Error())
		return err
	}

	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (cs *CredentialSpecResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(CredentialSpecCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (cs *CredentialSpecResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.knownStatusUnsafe = status
	cs.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (cs *CredentialSpecResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if cs.appliedStatus == resourcestatus.ResourceStatus(CredentialSpecStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if cs.appliedStatus <= knownStatus {
		cs.appliedStatus = resourcestatus.ResourceStatus(CredentialSpecStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (cs *CredentialSpecResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if cs.appliedStatus != resourcestatus.ResourceStatus(CredentialSpecStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	cs.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (cs *CredentialSpecResource) GetKnownStatus() resourcestatus.ResourceStatus {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (cs *CredentialSpecResource) StatusString(status resourcestatus.ResourceStatus) string {
	return CredentialSpecStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) GetCreatedAt() time.Time {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.createdAt
}

// getExecutionCredentialsID returns the execution role's credential ID
func (cs *CredentialSpecResource) getExecutionCredentialsID() string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.executionCredentialsID
}

// GetName safely returns the name of the resource
func (cs *CredentialSpecResource) GetName() string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return ResourceName
}

// Create is used to retrieve credentialspec resources for a given task
func (cs *CredentialSpecResource) Create() error {
	var iamCredentials credentials.IAMRoleCredentials

	executionCredentials, ok := cs.credentialsManager.GetTaskCredentials(cs.getExecutionCredentialsID())
	if ok {
		iamCredentials = executionCredentials.GetIAMRoleCredentials()
	}

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
		if parsedARNService == "secretsmanager" {
			err = cs.handleASMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from secretsmanager: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else if parsedARNService == "ssm" {
			err = cs.handleSSMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from SSM: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else {
			err = errors.New("unsupported credentialspec ARN, only secretsmanager/ssm ARNs are valid")
			cs.setTerminalReason(err.Error())
			return err
		}
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

func (cs *CredentialSpecResource) handleASMCredentialspecFile(originalCredentialspec, credentialspecASMARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecASMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	asmClient := cs.asmClientCreator.NewASMClient(cs.region, iamCredentials)

	// An ASM ARN is in the form of arn:aws:secretsmanager:us-west-2:secret:test-8mJ3EJ. The parsed ARN value
	// would be secret: test-8mJ3EJ. The following code gets the ASM secret by passing "test" value to the
	// GetSecretFromASM method to retrieve the value in the secrets store.
	asmArray := strings.SplitN(parsedARN.Resource, "secret:", 2)
	if len(asmArray) != 2 {
		err := fmt.Errorf("the provided ASM arn:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		return err
	}
	asmKey := strings.SplitN(asmArray[1], "-", 2)
	if len(asmKey) != 2 {
		err := fmt.Errorf("the provided ASM key:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		return err
	}
	key := asmKey[0]
	credSpecJsonString, err := asm.GetSecretFromASM(key, asmClient)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	cs.updateCredSpecMapping(originalCredentialspec, credSpecJsonString)

	return nil
}

func (cs *CredentialSpecResource) updateCredSpecMapping(credSpecInput, credSpecContent string) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	//parse json to extract the service account name and the domain name
	var jsonMap map[string]map[string]interface{}

	// Unmarshal or Decode the JSON to the interface.
	err := json.Unmarshal([]byte(credSpecContent), &jsonMap)

	if err != nil {
		seelog.Debugf("Error unmarshalling credentialspec data %s", credSpecContent)
	}

	serviceAccountName := jsonMap["DomainJoinConfig"]["MachineAccountName"].(string)
	domainName := jsonMap["DomainJoinConfig"]["DnsName"].(string)

	if len(serviceAccountName) != 0 && len(domainName) != 0 {
		cs.ServiceAccountInfoMap[credSpecInput] = ServiceAccountInfo{
			serviceAccountName: serviceAccountName,
			domainName:         domainName,
		}

		//build request array for credentials fetcher daemon
		cs.credentialsFetcherRequest = append(cs.credentialsFetcherRequest, credSpecContent)
	}
}

func (cs *CredentialSpecResource) handleSSMCredentialspecFile(originalCredentialspec, credentialspecSSMARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecSSMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmClient := cs.ssmClientCreator.NewSSMClient(cs.region, iamCredentials)

	// An SSM ARN is in the form of arn:aws:ssm:us-west-2:123456789012:parameter/a/b. The parsed ARN value
	// would be parameter/a/b. The following code gets the SSM parameter by passing "/a/b" value to the
	// GetParametersFromSSM method to retrieve the value in the parameter.
	ssmParam := strings.SplitAfterN(parsedARN.Resource, "parameter", 2)
	if len(ssmParam) != 2 {
		err := fmt.Errorf("the provided SSM parameter:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		return err
	}
	ssmParams := []string{ssmParam[1]}
	ssmParamMap, err := ssm.GetParametersFromSSM(ssmParams, ssmClient)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmParamData := ssmParamMap[ssmParam[1]]
	cs.updateCredSpecMapping(originalCredentialspec, ssmParamData)

	return nil
}

func (cs *CredentialSpecResource) GetTargetMapping(credSpecInput string) (string, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	targetCredSpecMapping, ok := cs.CredSpecMap[credSpecInput]
	if !ok {
		return "", errors.New("unable to obtain credentialspec mapping")
	}

	return targetCredSpecMapping, nil
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

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSON struct {
	TaskARN                    string                `json:"taskARN"`
	CreatedAt                  *time.Time            `json:"createdAt,omitempty"`
	DesiredStatus              *CredentialSpecStatus `json:"desiredStatus"`
	KnownStatus                *CredentialSpecStatus `json:"knownStatus"`
	CredentialSpecContainerMap map[string]string     `json:"CredentialSpecContainerMap"`
	CredSpecMap                map[string]string     `json:"CredSpecMap"`
	ExecutionCredentialsID     string                `json:"executionCredentialsID"`
}

// MarshalJSON serialises the CredentialSpecResourceJSON struct to JSON
func (cs *CredentialSpecResource) MarshalJSON() ([]byte, error) {
	if cs == nil {
		return nil, errors.New("credential specresource is nil")
	}
	createdAt := cs.GetCreatedAt()
	return json.Marshal(CredentialSpecResourceJSON{
		TaskARN:   cs.taskARN,
		CreatedAt: &createdAt,
		DesiredStatus: func() *CredentialSpecStatus {
			desiredState := cs.GetDesiredStatus()
			s := CredentialSpecStatus(desiredState)
			return &s
		}(),
		KnownStatus: func() *CredentialSpecStatus {
			knownState := cs.GetKnownStatus()
			s := CredentialSpecStatus(knownState)
			return &s
		}(),
		CredentialSpecContainerMap: cs.credentialSpecContainerMap,
		CredSpecMap:                cs.getCredSpecMap(),
		ExecutionCredentialsID:     cs.getExecutionCredentialsID(),
	})
}

func (cs *CredentialSpecResource) getCredSpecMap() map[string]string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.CredSpecMap
}

// UnmarshalJSON deserialises the raw JSON to a CredentialSpecResourceJSON struct
func (cs *CredentialSpecResource) UnmarshalJSON(b []byte) error {
	temp := CredentialSpecResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	if temp.DesiredStatus != nil {
		cs.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		cs.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		cs.SetCreatedAt(*temp.CreatedAt)
	}
	if temp.CredentialSpecContainerMap != nil {
		cs.credentialSpecContainerMap = temp.CredentialSpecContainerMap
	}
	if temp.CredSpecMap != nil {
		cs.CredSpecMap = temp.CredSpecMap
	}
	cs.taskARN = temp.TaskARN
	cs.executionCredentialsID = temp.ExecutionCredentialsID

	return nil
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (cs *CredentialSpecResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

func (cs *CredentialSpecResource) DependOnTaskNetwork() bool {
	return false
}

func (cs *CredentialSpecResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (cs *CredentialSpecResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
