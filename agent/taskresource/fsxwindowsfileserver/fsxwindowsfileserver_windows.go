//go:build windows
// +build windows

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

package fsxwindowsfileserver

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws/arn"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/fsx"
	fsxfactory "github.com/aws/amazon-ecs-agent/agent/fsx/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	resourceProvisioningError = "VolumeError: Agent could not create task's volume resources"
)

// FSxWindowsFileServerResource represents a fsxwindowsfileserver resource
type FSxWindowsFileServerResource struct {
	Name                   string
	VolumeType             string
	VolumeConfig           FSxWindowsFileServerVolumeConfig
	taskARN                string
	region                 string
	executionCredentialsID string
	credentialsManager     credentials.Manager

	// ssmClientCreator is a factory interface that creates new SSM clients.
	ssmClientCreator ssmfactory.SSMClientCreator
	// asmClientCreator is a factory interface that creates new ASM clients.
	asmClientCreator asmfactory.ClientCreator
	// fsxClientCreator is a factory interface that creates new FSx clients.
	fsxClientCreator fsxfactory.FSxClientCreator

	// fields that are set later during resource creation
	FSxWindowsFileServerDNSName string
	Credentials                 FSxWindowsFileServerCredentials

	// Fields for the common functionality of task resource. Access to these fields are protected by lock.
	createdAtUnsafe     time.Time
	knownStatusUnsafe   resourcestatus.ResourceStatus
	desiredStatusUnsafe resourcestatus.ResourceStatus
	appliedStatusUnsafe resourcestatus.ResourceStatus
	statusToTransitions map[resourcestatus.ResourceStatus]func() error
	terminalReason      string
	terminalReasonOnce  sync.Once
	lock                sync.RWMutex
}

// FSxWindowsFileServerVolumeConfig represents fsxWindowsFileServer volume configuration.
type FSxWindowsFileServerVolumeConfig struct {
	FileSystemID  string                         `json:"fileSystemId,omitempty"`
	RootDirectory string                         `json:"rootDirectory,omitempty"`
	AuthConfig    FSxWindowsFileServerAuthConfig `json:"authorizationConfig,omitempty"`
	// HostPath is used for bind mount as part of HostConfig.
	HostPath string `json:"fsxWindowsFileServerHostPath"`
}

// FSxWindowsFileServerAuthConfig contains auth config for a fsxWindowsFileServer volume.
type FSxWindowsFileServerAuthConfig struct {
	CredentialsParameter string `json:"credentialsParameter,omitempty"`
	Domain               string `json:"domain,omitempty"`
}

// FSxWindowsFileServerCredentials represents user credentials for accessing the fsxWindowsFileServer volume
type FSxWindowsFileServerCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewFSxWindowsFileServerResource creates a new FSxWindowsFileServerResource object
func NewFSxWindowsFileServerResource(
	taskARN,
	region string,
	name string,
	volumeType string,
	volumeConfig *FSxWindowsFileServerVolumeConfig,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	fsxClientCreator fsxfactory.FSxClientCreator) (*FSxWindowsFileServerResource, error) {

	fv := &FSxWindowsFileServerResource{
		Name:       name,
		VolumeType: volumeType,
		VolumeConfig: FSxWindowsFileServerVolumeConfig{
			FileSystemID:  volumeConfig.FileSystemID,
			RootDirectory: volumeConfig.RootDirectory,
			AuthConfig:    volumeConfig.AuthConfig,
		},
		taskARN:                taskARN,
		region:                 region,
		executionCredentialsID: executionCredentialsID,
		credentialsManager:     credentialsManager,
		ssmClientCreator:       ssmClientCreator,
		asmClientCreator:       asmClientCreator,
		fsxClientCreator:       fsxClientCreator,
	}

	fv.initStatusToTransition()
	return fv, nil
}

func (fv *FSxWindowsFileServerResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {

	fv.credentialsManager = resourceFields.CredentialsManager
	fv.ssmClientCreator = resourceFields.SSMClientCreator
	fv.asmClientCreator = resourceFields.ASMClientCreator
	fv.fsxClientCreator = resourceFields.FSxClientCreator
	fv.initStatusToTransition()
}

func (fv *FSxWindowsFileServerResource) initStatusToTransition() {
	statusToTransitions := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeCreated): fv.Create,
	}

	fv.statusToTransitions = statusToTransitions
}

// DesiredTerminal returns true if the fsxwindowsfileserver's desired status is REMOVED
func (fv *FSxWindowsFileServerResource) DesiredTerminal() bool {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.desiredStatusUnsafe == resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeRemoved)
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (fv *FSxWindowsFileServerResource) GetTerminalReason() string {
	if fv.terminalReason == "" {
		return resourceProvisioningError
	}
	return fv.terminalReason
}

func (fv *FSxWindowsFileServerResource) setTerminalReason(reason string) {
	fv.terminalReasonOnce.Do(func() {
		seelog.Debugf("fsxwindowsfileserver resource [%s]: setting terminal reason for fsxwindowsfileserver resource in task: [%s]", fv.Name, fv.taskARN)
		fv.terminalReason = reason
	})
}

// GetDesiredStatus safely returns the desired status of the task
func (fv *FSxWindowsFileServerResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.desiredStatusUnsafe
}

// SetDesiredStatus safely sets the desired status of the resource
func (fv *FSxWindowsFileServerResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.desiredStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task
func (fv *FSxWindowsFileServerResource) GetKnownStatus() resourcestatus.ResourceStatus {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.knownStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource
func (fv *FSxWindowsFileServerResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.knownStatusUnsafe = status
	fv.updateAppliedStatusUnsafe(status)
}

// KnownCreated returns true if the fsxwindowsfileserver's known status is CREATED
func (fv *FSxWindowsFileServerResource) KnownCreated() bool {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.knownStatusUnsafe == resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeCreated)
}

// TerminalStatus returns the last transition state of fsxwindowsfileserver
func (fv *FSxWindowsFileServerResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (fv *FSxWindowsFileServerResource) NextKnownState() resourcestatus.ResourceStatus {
	return fv.GetKnownStatus() + 1
}

// SteadyState returns the transition state of the resource defined as "ready"
func (fv *FSxWindowsFileServerResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeCreated)
}

// ApplyTransition calls the function required to move to the specified status
func (fv *FSxWindowsFileServerResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := fv.statusToTransitions[nextState]
	if !ok {
		err := errors.Errorf("resource [%s]: transition to %s impossible", fv.Name,
			fv.StatusString(nextState))
		fv.setTerminalReason(err.Error())
		return err
	}

	return transitionFunc()
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (fv *FSxWindowsFileServerResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	if fv.appliedStatusUnsafe != resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	fv.appliedStatusUnsafe = status
	return true
}

// StatusString returns the string of the fsxwindowsfileserver resource status
func (fv *FSxWindowsFileServerResource) StatusString(status resourcestatus.ResourceStatus) string {
	return FSxWindowsFileServerVolumeStatus(status).String()
}

// GetCreatedAt gets the timestamp for resource's creation time
func (fv *FSxWindowsFileServerResource) GetCreatedAt() time.Time {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.createdAtUnsafe
}

// SetCreatedAt sets the timestamp for resource's creation time
func (fv *FSxWindowsFileServerResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.createdAtUnsafe = createdAt
}

// Source returns the host path of the fsxwindowsfileserver resource which is used as the source of the volume mount
func (cfg *FSxWindowsFileServerVolumeConfig) Source() string {
	return utils.GetCanonicalPath(cfg.HostPath)
}

// GetName safely returns the name of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) GetName() string {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.Name
}

// GetVolumeConfig safely returns the volume config of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) GetVolumeConfig() FSxWindowsFileServerVolumeConfig {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.VolumeConfig
}

// GetCredentials safely returns the user credentials of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) GetCredentials() FSxWindowsFileServerCredentials {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.Credentials
}

// GetFileSystemDNSName safely returns the filesystem dns name of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) GetFileSystemDNSName() string {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.FSxWindowsFileServerDNSName
}

// getExecutionCredentialsID safely returns the execution role's credential ID
func (fv *FSxWindowsFileServerResource) GetExecutionCredentialsID() string {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.executionCredentialsID
}

// SetCredentials safely updates the user credentials of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) SetCredentials(credentials FSxWindowsFileServerCredentials) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.Credentials = credentials
}

// SetFileSystemDNSName safely updates the filesystem dns name of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) SetFileSystemDNSName(DNSName string) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.FSxWindowsFileServerDNSName = DNSName
}

// SetFileSystemDNSName safely updates volume config's root directory of the fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) SetRootDirectory(rootDirectory string) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.VolumeConfig.RootDirectory = rootDirectory
}

// GetHostPath safely returns the volume config's host path
func (fv *FSxWindowsFileServerResource) GetHostPath() string {
	fv.lock.RLock()
	defer fv.lock.RUnlock()

	return fv.VolumeConfig.HostPath
}

// SetHostPath safely updates volume config's host path
func (fv *FSxWindowsFileServerResource) SetHostPath(hostPath string) {
	fv.lock.Lock()
	defer fv.lock.Unlock()

	fv.VolumeConfig.HostPath = hostPath
}

// Create is used to create all the fsxwindowsfileserver resources for a given task
func (fv *FSxWindowsFileServerResource) Create() error {
	var err error
	volumeConfig := fv.GetVolumeConfig()

	var iamCredentials credentials.IAMRoleCredentials
	executionCredentials, ok := fv.credentialsManager.GetTaskCredentials(fv.GetExecutionCredentialsID())
	if !ok {
		err = errors.New("fsxwindowsfileserver resource: unable to find execution role credentials")
		fv.setTerminalReason(err.Error())
		return err
	}
	iamCredentials = executionCredentials.GetIAMRoleCredentials()

	err = fv.retrieveCredentials(volumeConfig.AuthConfig.CredentialsParameter, iamCredentials)
	if err != nil {
		fv.setTerminalReason(err.Error())
		return err
	}

	err = fv.retrieveFileSystemDNSName(volumeConfig.FileSystemID, iamCredentials)
	if err != nil {
		fv.setTerminalReason(err.Error())
		return err
	}

	fv.handleRootDirectory(volumeConfig.RootDirectory)

	creds := fv.GetCredentials()
	if creds.Username == "" || creds.Password == "" {
		return errors.New("invalid credentials for mounting fsxwindowsfileserver volume on the container instance")
	}
	// formatting to keep powershell happy
	username := volumeConfig.AuthConfig.Domain + `\` + creds.Username
	remotePath := fmt.Sprintf(`\\%s`, fv.GetFileSystemDNSName())
	if volumeConfig.RootDirectory != "" {
		remotePath = fmt.Sprintf(`%s\%s`, remotePath, volumeConfig.RootDirectory)
	} else {
		remotePath = fmt.Sprintf(`%s\share`, remotePath)
	}

	password := creds.Password

	err = fv.performHostMount(remotePath, username, password)
	if err != nil {
		fv.setTerminalReason(err.Error())
		return err
	}

	return nil
}

func (fv *FSxWindowsFileServerResource) retrieveCredentials(credentialsParameterARN string, iamCredentials credentials.IAMRoleCredentials) error {
	parsedARN, err := arn.Parse(credentialsParameterARN)
	if err != nil {
		fv.setTerminalReason(err.Error())
		return err
	}

	parsedARNService := parsedARN.Service
	switch parsedARNService {
	case "ssm":
		err = fv.retrieveSSMCredentials(credentialsParameterARN, iamCredentials)
		if err != nil {
			seelog.Errorf("Failed to retrieve credentials from ssm: %v", err)
			fv.setTerminalReason(err.Error())
			return err
		}
	case "secretsmanager":
		err = fv.retrieveASMCredentials(credentialsParameterARN, iamCredentials)
		if err != nil {
			seelog.Errorf("Failed to retrieve credentials from asm: %v", err)
			fv.setTerminalReason(err.Error())
			return err
		}
	default:
		err = errors.New("unsupported credentialsParameter, only ssm/secretsmanager ARNs are valid")
		fv.setTerminalReason(err.Error())
		return err
	}

	return nil
}

func (fv *FSxWindowsFileServerResource) retrieveSSMCredentials(credentialsParameterARN string, iamCredentials credentials.IAMRoleCredentials) error {
	parsedARN, err := arn.Parse(credentialsParameterARN)
	if err != nil {
		return err
	}

	ssmClient := fv.ssmClientCreator.NewSSMClient(fv.region, iamCredentials)
	ssmParam := filepath.Base(parsedARN.Resource)
	ssmParams := []string{ssmParam}

	ssmParamMap, err := ssm.GetParametersFromSSM(ssmParams, ssmClient)
	if err != nil {
		return err
	}

	ssmParamData, _ := ssmParamMap[ssmParam]
	creds := FSxWindowsFileServerCredentials{}

	if err := json.Unmarshal([]byte(ssmParamData), &creds); err != nil {
		return err
	}

	fv.SetCredentials(creds)
	return nil
}

func (fv *FSxWindowsFileServerResource) retrieveASMCredentials(credentialsParameterARN string, iamCredentials credentials.IAMRoleCredentials) error {
	_, err := arn.Parse(credentialsParameterARN)
	if err != nil {
		return err
	}

	asmClient := fv.asmClientCreator.NewASMClient(fv.region, iamCredentials)
	asmData, err := asm.GetSecretFromASM(credentialsParameterARN, asmClient)
	if err != nil {
		return err
	}

	creds := FSxWindowsFileServerCredentials{}
	if err := json.Unmarshal([]byte(asmData), &creds); err != nil {
		return err
	}

	fv.SetCredentials(creds)
	return nil
}

func (fv *FSxWindowsFileServerResource) retrieveFileSystemDNSName(fileSystemId string, iamCredentials credentials.IAMRoleCredentials) error {
	fileSystemIds := []string{fileSystemId}
	fsxClient := fv.fsxClientCreator.NewFSxClient(fv.region, iamCredentials)
	fileSystemDNSMap, err := fsx.GetFileSystemDNSNames(fileSystemIds, fsxClient)
	if err != nil {
		fv.setTerminalReason(err.Error())
		return err
	}

	fv.SetFileSystemDNSName(fileSystemDNSMap[fileSystemId])
	return nil
}

var mountLock sync.Mutex
var DriveLetterAvailable = utils.IsAvailableDriveLetter
var execCommand = exec.Command

func (fv *FSxWindowsFileServerResource) performHostMount(remotePath string, username string, password string) error {
	// mountLock is a package-level mutex lock.
	// It is used to avoid a scenario where concurrent tasks get the same drive letter and perform host mount at the same time.
	// A drive letter is free until host mount is performed, a lock prevents multiple tasks getting the same drive letter.
	mountLock.Lock()
	defer mountLock.Unlock()

	hostPath, err := utils.FindUnusedDriveLetter()
	if err != nil {
		return err
	}
	fv.SetHostPath(hostPath)

	// formatting to keep powershell happy
	localPathArg := fmt.Sprintf("-LocalPath \"%s\"", strings.Trim(hostPath, `\`))

	// remove mount if localPath is not available
	// handling case where agent crashes after mount and before resource status is updated
	if !(DriveLetterAvailable(hostPath)) {
		err = fv.removeHostMount(localPathArg)
		if err != nil {
			seelog.Errorf("Failed to remove existing fsxwindowsfileserver resource mount on the container instance: %v", err)
			fv.setTerminalReason(err.Error())
			return err
		}
	}

	// formatting to keep powershell happy
	creds := fmt.Sprintf("-Credential $(New-Object System.Management.Automation.PSCredential(\"%s\", $(ConvertTo-SecureString \"%s\" -AsPlainText -Force)))", username, password)
	remotePathArg := fmt.Sprintf("-RemotePath \"%s\"", remotePath)

	// New-SmbGlobalMapping cmdlet creates an SMB mapping between the container instance
	// and SMB share (FSx for Windows File Server file-system)
	cmd := execCommand("powershell.exe",
		"New-SmbGlobalMapping",
		localPathArg,
		remotePathArg,
		creds,
		"-Persistent $true",
		"-RequirePrivacy $true",
		"-ErrorAction Stop")

	_, err = cmd.CombinedOutput()
	if err != nil {
		seelog.Errorf("Failed to map fsxwindowsfileserver resource on the container instance: %v", err)
		fv.setTerminalReason(err.Error())
		return err
	}

	// Get-SmbGlobalMapping cmdlet retrieves existing SMB mappings
	cmd = execCommand("powershell.exe", "Get-SmbGlobalMapping", localPathArg)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return errors.New("failed to verify fsxwindowsfileserver resource map on the container instance")
	}

	return nil
}

func (fv *FSxWindowsFileServerResource) handleRootDirectory(rootDirectory string) {
	dir := utils.GetCanonicalPath(rootDirectory)
	dir = strings.Trim(dir, "\\")

	fv.SetRootDirectory(dir)
	return
}

func (fv *FSxWindowsFileServerResource) removeHostMount(localPathArg string) error {
	// Remove-SmbGlobalMapping cmdlet removes the existing Server Message Block (SMB) mapping between the
	// container instance and SMB share (FSx for Windows File Server file-system)
	cmd := execCommand("powershell.exe", "Remove-SmbGlobalMapping", localPathArg, "-Force")

	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func (fv *FSxWindowsFileServerResource) Cleanup() error {
	localPath := fv.GetHostPath()
	// formatting to keep powershell happy
	localPathArg := fmt.Sprintf("-LocalPath \"%s\"", strings.Trim(localPath, `\`))
	err := fv.removeHostMount(localPathArg)
	if err != nil {
		seelog.Warnf("Unable to clear fsxwindowsfileserver host path %s for task %s: %s", localPath, fv.taskARN, err)
	}
	return nil
}

// FSxWindowsFileServerResourceJSON is the json representation of the fsxwindowsfileserver resource
type FSxWindowsFileServerResourceJSON struct {
	Name                   string                            `json:"name"`
	VolumeConfig           FSxWindowsFileServerVolumeConfig  `json:"fsxWindowsFileServerVolumeConfiguration"`
	TaskARN                string                            `json:"taskARN"`
	ExecutionCredentialsID string                            `json:"executionCredentialsID"`
	CreatedAt              *time.Time                        `json:"createdAt,omitempty"`
	DesiredStatus          *FSxWindowsFileServerVolumeStatus `json:"desiredStatus"`
	KnownStatus            *FSxWindowsFileServerVolumeStatus `json:"knownStatus"`
}

// MarshalJSON serialises the FSxWindowsFileServerResourceJSON struct to JSON
func (fv *FSxWindowsFileServerResource) MarshalJSON() ([]byte, error) {
	if fv == nil {
		return nil, errors.New("fsxwindowfileserver resource is nil")
	}
	createdAt := fv.GetCreatedAt()
	return json.Marshal(FSxWindowsFileServerResourceJSON{
		Name:                   fv.Name,
		VolumeConfig:           fv.VolumeConfig,
		TaskARN:                fv.taskARN,
		ExecutionCredentialsID: fv.executionCredentialsID,
		CreatedAt:              &createdAt,
		DesiredStatus: func() *FSxWindowsFileServerVolumeStatus {
			desiredState := fv.GetDesiredStatus()
			s := FSxWindowsFileServerVolumeStatus(desiredState)
			return &s
		}(),
		KnownStatus: func() *FSxWindowsFileServerVolumeStatus {
			knownState := fv.GetKnownStatus()
			s := FSxWindowsFileServerVolumeStatus(knownState)
			return &s
		}(),
	})
}

// UnmarshalJSON deserialises the raw JSON to a FSxWindowsFileServerResourceJSON struct
func (fv *FSxWindowsFileServerResource) UnmarshalJSON(b []byte) error {
	temp := FSxWindowsFileServerResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	fv.Name = temp.Name
	fv.VolumeConfig = temp.VolumeConfig
	fv.taskARN = temp.TaskARN
	fv.executionCredentialsID = temp.ExecutionCredentialsID
	if temp.DesiredStatus != nil {
		fv.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		fv.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		fv.SetCreatedAt(*temp.CreatedAt)
	}
	return nil
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (fv *FSxWindowsFileServerResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if fv.appliedStatusUnsafe == resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if fv.appliedStatusUnsafe <= knownStatus {
		fv.appliedStatusUnsafe = resourcestatus.ResourceStatus(FSxWindowsFileServerVolumeStatusNone)
	}
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (fv *FSxWindowsFileServerResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	fv.lock.RLock()
	defer fv.lock.RLock()

	return fv.appliedStatusUnsafe
}

func (fv *FSxWindowsFileServerResource) DependOnTaskNetwork() bool {
	return false
}

// BuildContainerDependency sets the container dependencies of the resource.
func (fv *FSxWindowsFileServerResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
	return
}

// GetContainerDependencies returns the container dependencies of the resource.
func (fv *FSxWindowsFileServerResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
