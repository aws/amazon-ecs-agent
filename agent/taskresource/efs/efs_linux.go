// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package efs

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// ResourceName is the name of the efs resource
	ResourceName = "efs"
	// netnsFormat is used to construct the path to container network namespace
	netnsFormat = "/host/proc/%s/ns/net"
	// The root directory to create mount target directory under
	DirPath = "/efs"
)

var (
	lookUpHostHelper = net.LookupHost
)

// EFSConfig represents efs volume configuration
type EFSConfig struct {
	FileSystem    string   `json:"fileSystem"`
	TargetDir     string   `json:"targetDirectory"`
	RootDir       string   `json:"rootDirectory"`
	ReadOnly      bool     `json:"readOnly"`
	DNSEndpoints  []string `json:"dnsEndpoints"`
	MountOptions  string   `json:"nfsMountOptions"`
	DataDirOnHost string   `json:"dataDirOnHost"`
}

func (efsConfig *EFSConfig) Source() string {
	strs := strings.Split(efsConfig.TargetDir, "/")
	path := filepath.Join(efsConfig.DataDirOnHost, DirPath, strs[len(strs)-1])
	return path
}

// EFSResource handles EFS file system mount/unmount and tunnel service start/stop.
type EFSResource struct {
	taskARN                            string
	volumeName                         string
	os                                 oswrapper.OS
	ctx                                context.Context
	client                             dockerapi.DockerClient
	pauseContainerName                 string
	awsvpcMode                         bool
	Config                             EFSConfig
	nfsMount                           *NFSMount
	createdAt                          time.Time
	desiredStatusUnsafe                resourcestatus.ResourceStatus
	knownStatusUnsafe                  resourcestatus.ResourceStatus
	appliedStatusUnsafe                resourcestatus.ResourceStatus
	resourceStatusToTransitionFunction map[resourcestatus.ResourceStatus]func() error

	// TransitionDependenciesMap is a map of the dependent container status to other
	// dependencies that must be satisfied in order for this container to transition.
	transitionDependenciesMap taskresource.TransitionDependenciesMap

	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisioning failed.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewEFSResource creates a new EFSResource object
func NewEFSResource(taskARN string,
	volumeName string,
	ctx context.Context,
	client dockerapi.DockerClient,
	awsvpc bool,
	targetDir string,
	rootDir string,
	dnsEndpoints []string,
	readOnly bool,
	mountOptions string,
	dataDirOnHost string) *EFSResource {

	s := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		ctx:        ctx,
		client:     client,
		awsvpcMode: awsvpc,
		Config: EFSConfig{
			TargetDir:     targetDir,
			RootDir:       rootDir,
			DNSEndpoints:  dnsEndpoints,
			MountOptions:  mountOptions,
			ReadOnly:      readOnly,
			DataDirOnHost: dataDirOnHost,
		},
		os:                        oswrapper.NewOS(),
		transitionDependenciesMap: make(map[resourcestatus.ResourceStatus]taskresource.TransitionDependencySet),
	}

	s.initStatusToTransition()
	return s
}

func (efs *EFSResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(EFSCreated): efs.Create,
		resourcestatus.ResourceStatus(EFSRemoved): efs.Cleanup,
	}
	efs.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (efs *EFSResource) setTerminalReason(reason string) {
	efs.terminalReasonOnce.Do(func() {
		seelog.Infof("efs resource: setting terminal reason for efs resource in task: [%s]", efs.taskARN)
		efs.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (efs *EFSResource) GetTerminalReason() string {
	return efs.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource
func (efs *EFSResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (efs *EFSResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (efs *EFSResource) GetName() string {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.volumeName
}

// DesiredTerminal returns true if the efs's desired status is REMOVED
func (efs *EFSResource) DesiredTerminal() bool {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.desiredStatusUnsafe == resourcestatus.ResourceStatus(EFSRemoved)
}

// KnownCreated returns true if the efs's known status is CREATED
func (efs *EFSResource) KnownCreated() bool {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.knownStatusUnsafe == resourcestatus.ResourceStatus(EFSCreated)
}

// TerminalStatus returns the last transition state of efs
func (efs *EFSResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(EFSRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (efs *EFSResource) NextKnownState() resourcestatus.ResourceStatus {
	return efs.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (efs *EFSResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := efs.resourceStatusToTransitionFunction[nextState]
	if !ok {
		errW := errors.Errorf("efs [%s]: transition to %s impossible", efs.volumeName,
			efs.StatusString(nextState))
		efs.setTerminalReason(errW.Error())
		return errW
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (efs *EFSResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(EFSRemoved)
}

// SetKnownStatus safely sets the currently known status of the resource
func (efs *EFSResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.knownStatusUnsafe = status
	efs.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (efs *EFSResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if efs.appliedStatusUnsafe == resourcestatus.ResourceStatus(EFSStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if efs.appliedStatusUnsafe <= knownStatus {
		efs.appliedStatusUnsafe = resourcestatus.ResourceStatus(EFSStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (efs *EFSResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	if efs.appliedStatusUnsafe != resourcestatus.ResourceStatus(EFSStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	efs.appliedStatusUnsafe = status
	return true
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (efs *EFSResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.appliedStatusUnsafe
}

// GetKnownStatus safely returns the currently known status of the resource
func (efs *EFSResource) GetKnownStatus() resourcestatus.ResourceStatus {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.knownStatusUnsafe
}

// StatusString returns the string of the efs resource status
func (efs *EFSResource) StatusString(status resourcestatus.ResourceStatus) string {
	return EFSStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (efs *EFSResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (efs *EFSResource) GetCreatedAt() time.Time {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.createdAt
}

// Create create mount target directory, resolve the dns of the EFS file system and mount the file system.
func (efs *EFSResource) Create() error {

	// Create mount target directory.
	if err := efs.createDir(); err != nil {
		efs.setTerminalReason(err.Error())
		return err
	}

	efsMount, err := efs.initMount()
	if err != nil {
		efs.setTerminalReason(err.Error())
		return err
	}
	efs.setNFSMount(efsMount)

	err = efsMount.Mount()
	// if mount returns "already mounted" error, log error message as warning and continue
	if err != nil {
		if strings.Contains(err.Error(), "already mounted") {
			seelog.Warnf("mount efs resource: %s", err.Error())
		} else {
			errW := errors.Wrap(err, "unable to mount efs")
			efs.setTerminalReason(errW.Error())
			return errW
		}
	}

	return nil
}

// initMount initializes the NFSMount
func (efs *EFSResource) initMount() (*NFSMount, error) {
	netns := ""
	// Get the task network namespace if under awsvpc mode.
	if efs.awsvpcMode {
		containerInspectOutput, err := efs.client.InspectContainer(
			efs.ctx,
			efs.pauseContainerName,
			dockerclient.InspectContainerTimeout,
		)

		if err != nil {
			return nil, errors.Wrap(err, "efs inspect container failed")
		}

		containerPID := strconv.Itoa(containerInspectOutput.State.Pid)
		netns = fmt.Sprintf(netnsFormat, containerPID)
	}

	// Mount EFS, resolve DNS to get the ip address first
	ip, err := efs.dnsResolve()
	if err != nil {
		return nil, err
	}
	efsMount := &NFSMount{
		IPAddress:       ip,
		TargetDirectory: efs.Config.TargetDir,
		SourceDirectory: efs.Config.RootDir,
		NamespacePath:   netns,
	}
	return efsMount, nil
}

// createDir creates mount target directory
func (efs *EFSResource) createDir() error {
	// if path already exists, MkdirAll returns nil
	err := efs.os.MkdirAll(efs.Config.TargetDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create mount target directory for efs")
	}

	return nil
}

// removeDir removes mount target directory
func (efs *EFSResource) removeDir() error {
	err := efs.os.RemoveAll(efs.Config.TargetDir)
	if err != nil {
		return errors.Wrap(err, "unable to remove mount target directory for efs")
	}

	return nil
}

func (efs *EFSResource) dnsResolve() (string, error) {
	ips, err := lookUpHostHelper(efs.Config.DNSEndpoints[0])
	if err != nil {
		return "", errors.Wrap(err, "unable to resolve dns for efs")
	}

	return ips[0], nil
}

func (efs *EFSResource) setNFSMount(nfs *NFSMount) {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.nfsMount = nfs
}

func (efs *EFSResource) getNFSMount() *NFSMount {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return efs.nfsMount
}

func (efs *EFSResource) setOS() {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	if efs.os == nil {
		efs.os = oswrapper.NewOS()
	}
}

// Cleanup unmount the EFS and remove the mount target directory
func (efs *EFSResource) Cleanup() error {
	var err error

	// unmount the EFS file system
	efsMount := efs.getNFSMount()
	if efsMount == nil {
		if efsMount, err = efs.initMount(); err != nil {
			efs.setTerminalReason(err.Error())
			return err
		}
	}

	err = efsMount.Unmount()
	if err != nil && !strings.Contains(err.Error(), "not mounted") {
		errW := errors.Wrap(err, "unable to unmount efs")
		efs.setTerminalReason(errW.Error())
		return errW
	}

	// remove the mount target directory
	if err = efs.removeDir(); err != nil {
		efs.setTerminalReason(err.Error())
		return err
	}

	return nil
}

func (efs *EFSResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {

	efs.initStatusToTransition()
	efs.setOS()
}

type efsResourceJSON struct {
	TaskARN                   string                                 `json:"taskARN"`
	VolumeName                string                                 `json:"volumeName"`
	AwsvpcMode                bool                                   `json:"awsvpcMode"`
	PauseContainerName        string                                 `json:"pauseContainerName"`
	Config                    EFSConfig                              `json:"efsConfig"`
	TransitionDependenciesMap taskresource.TransitionDependenciesMap `json:"transitionDependenciesMap"`
	TerminalReason            string                                 `json:"terminalReason"`

	CreatedAt     time.Time  `json:"createdAt"`
	DesiredStatus *EFSStatus `json:"desiredStatus"`
	KnownStatus   *EFSStatus `json:"knownStatus"`
}

// MarshalJSON marshals EFSResource object using duplicate struct EFSResourceJSON
func (efs *EFSResource) MarshalJSON() ([]byte, error) {
	if efs == nil {
		return nil, errors.New("efs resource is nil")
	}

	efs.lock.RLock()
	defer efs.lock.RUnlock()

	return json.Marshal(efsResourceJSON{
		TaskARN:                   efs.taskARN,
		VolumeName:                efs.volumeName,
		AwsvpcMode:                efs.awsvpcMode,
		PauseContainerName:        efs.pauseContainerName,
		Config:                    efs.Config,
		TransitionDependenciesMap: efs.transitionDependenciesMap,
		TerminalReason:            efs.terminalReason,
		CreatedAt:                 efs.createdAt,
		DesiredStatus: func() *EFSStatus {
			desiredState := efs.GetDesiredStatus()
			s := EFSStatus(desiredState)
			return &s
		}(),
		KnownStatus: func() *EFSStatus {
			knownState := efs.GetKnownStatus()
			s := EFSStatus(knownState)
			return &s
		}(),
	})
}

// UnmarshalJSON unmarshals EFSResource object using duplicate struct EFSResourceJSON
func (efs *EFSResource) UnmarshalJSON(b []byte) error {
	temp := &efsResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.taskARN = temp.TaskARN
	efs.volumeName = temp.VolumeName
	efs.Config = temp.Config
	efs.awsvpcMode = temp.AwsvpcMode
	efs.transitionDependenciesMap = temp.TransitionDependenciesMap
	efs.pauseContainerName = temp.PauseContainerName
	efs.terminalReason = temp.TerminalReason
	efs.createdAt = temp.CreatedAt

	if temp.DesiredStatus != nil {
		efs.desiredStatusUnsafe = resourcestatus.ResourceStatus(*temp.DesiredStatus)
	}
	if temp.KnownStatus != nil {
		efs.knownStatusUnsafe = resourcestatus.ResourceStatus(*temp.KnownStatus)
	}
	return nil
}

// DependOnTaskNetwork shows whether the resource creation needs task network setup beforehand
func (efs *EFSResource) DependOnTaskNetwork() bool {
	return true
}

func (efs *EFSResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) error {

	contDep := apicontainer.ContainerDependency{
		ContainerName:   containerName,
		SatisfiedStatus: satisfied,
	}
	if _, ok := efs.transitionDependenciesMap[dependent]; !ok {
		efs.transitionDependenciesMap[dependent] = taskresource.TransitionDependencySet{}
	}
	deps := efs.transitionDependenciesMap[dependent]
	deps.ContainerDependencies = append(deps.ContainerDependencies, contDep)
	efs.transitionDependenciesMap[dependent] = deps
	return nil
}

func (efs *EFSResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	if _, ok := efs.transitionDependenciesMap[dependent]; !ok {
		return nil
	}
	return efs.transitionDependenciesMap[dependent].ContainerDependencies
}

// SetPauseContainerName is used to set the value of field pauseContainerName when creating container, since the container
// name is generated during that time
func (efs *EFSResource) SetPauseContainerName(name string) {
	efs.lock.Lock()
	defer efs.lock.Unlock()

	efs.pauseContainerName = name
}

// GetPauseContainerName returns pauseContainerName, here is only for testing use.
func (efs *EFSResource) GetPauseContainerName() string {
	return efs.pauseContainerName
}

// UpdateAppliedStatus safely updates the applied status of the resource
func (efs *EFSResource) UpdateAppliedStatus(status resourcestatus.ResourceStatus) {
	efs.lock.RLock()
	defer efs.lock.RUnlock()

	efs.appliedStatusUnsafe = status
}
