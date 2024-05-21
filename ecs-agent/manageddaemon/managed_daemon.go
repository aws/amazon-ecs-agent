// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package manageddaemon

import (
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"

	dockercontainer "github.com/docker/docker/api/types/container"
)

type ManagedDaemon struct {
	imageName string
	imageTag  string

	healthCheckTest     []string
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	healthCheckRetries  int

	// Daemons require an agent <-> daemon mount
	// identified by the volume name `agentCommunicationMount`
	// the SourceVolumeHostPath will always be overridden to
	// /var/run/ecs/<md.imageName>
	agentCommunicationMount *MountPoint

	// Daemons require an application log mount
	// identified by the volume name `applicationLogMount`
	// the SourceVolumeHostPath will always be overridden to
	// /var/log/ecs/<md.imageName>
	applicationLogMount *MountPoint

	mountPoints []*MountPoint
	environment map[string]string

	loadedDaemonImageRef string
	command              []string

	linuxParameters *ecsacs.LinuxParameters
	privileged      bool

	containerId string

	containerCGroup  string
	networkNameSpace string
}

// A valid managed daemon will require
// healthcheck and mount points to be added
func NewManagedDaemon(
	imageName string,
	imageTag string,
) *ManagedDaemon {
	if imageTag == "" {
		imageTag = imageTagDefault
	}
	// health check retries 0 is valid
	newManagedDaemon := &ManagedDaemon{
		imageName:          imageName,
		imageTag:           imageTag,
		healthCheckRetries: 0,
	}
	return newManagedDaemon
}

var ImportAll = defaultImportAll

func (md *ManagedDaemon) GetLinuxParameters() *ecsacs.LinuxParameters {
	return md.linuxParameters
}

func (md *ManagedDaemon) GetPrivileged() bool {
	return md.privileged
}

func (md *ManagedDaemon) GetImageName() string {
	return md.imageName
}

func (md *ManagedDaemon) GetImageTag() string {
	return md.imageTag
}

func (md *ManagedDaemon) GetImageRef() string {
	return (fmt.Sprintf("%s:%s", md.imageName, md.imageTag))
}

func (md *ManagedDaemon) GetImageTarPath() string {
	return (fmt.Sprintf("%s/%s/%s.tar", imageTarPath, md.imageName, md.imageName))
}

func (md *ManagedDaemon) GetAgentCommunicationMount() *MountPoint {
	return md.agentCommunicationMount
}

func (md *ManagedDaemon) GetApplicationLogMount() *MountPoint {
	return md.applicationLogMount
}

func (md *ManagedDaemon) GetCommand() []string {
	return md.command
}

func (md *ManagedDaemon) SetCommand(command []string) {
	md.command = make([]string, len(command))
	copy(md.command, command)
}

// returns list of mountpoints without the
// agentCommunicationMount and applicationLogMount
func (md *ManagedDaemon) GetFilteredMountPoints() []*MountPoint {
	filteredMounts := make([]*MountPoint, len(md.mountPoints))
	copy(filteredMounts, md.mountPoints)
	return filteredMounts
}

// returns list of mountpoints which (re)integrates
// agentCommunicationMount and applicationLogMount
// these will always include host mount file overrides
func (md *ManagedDaemon) GetMountPoints() []*MountPoint {
	allMounts := make([]*MountPoint, len(md.mountPoints))
	copy(allMounts, md.mountPoints)
	allMounts = append(allMounts, md.agentCommunicationMount)
	allMounts = append(allMounts, md.applicationLogMount)
	return allMounts
}

func (md *ManagedDaemon) GetEnvironment() map[string]string {
	return md.environment
}

func (md *ManagedDaemon) GetLoadedDaemonImageRef() string {
	return md.loadedDaemonImageRef
}

func (md *ManagedDaemon) SetLoadedDaemonImageRef(loadedImageRef string) {
	md.loadedDaemonImageRef = loadedImageRef
}

func (md *ManagedDaemon) GetHealthCheckTest() []string {
	return md.healthCheckTest
}

func (md *ManagedDaemon) GetHealthCheckInterval() time.Duration {
	return md.healthCheckInterval
}

func (md *ManagedDaemon) GetHealthCheckTimeout() time.Duration {
	return md.healthCheckTimeout
}

func (md *ManagedDaemon) GetHealthCheckRetries() int {
	return md.healthCheckRetries
}

func (md *ManagedDaemon) SetHealthCheck(
	healthCheckTest []string,
	healthCheckInterval time.Duration,
	healthCheckTimeout time.Duration,
	healthCheckRetries int) {
	md.healthCheckInterval = healthCheckInterval
	md.healthCheckTimeout = healthCheckTimeout
	md.healthCheckRetries = healthCheckRetries
	md.healthCheckTest = make([]string, len(healthCheckTest))
	copy(md.healthCheckTest, healthCheckTest)
}

// filter mount points for agentCommunicationMount
// set required mounts
// and override host paths in favor of agent defaults
// when a duplicate SourceVolumeID is given, the last Mount wins
func (md *ManagedDaemon) SetMountPoints(mountPoints []*MountPoint) error {
	var mountPointMap = make(map[string]*MountPoint)
	for _, mp := range mountPoints {
		if mp.SourceVolumeID == defaultAgentCommunicationMount {
			mp.SourceVolumeHostPath = fmt.Sprintf("%s/%s/", defaultAgentCommunicationPathHostRoot, md.imageName)
			md.agentCommunicationMount = mp
		} else if mp.SourceVolumeID == defaultApplicationLogMount {
			mp.SourceVolumeHostPath = fmt.Sprintf("%s/%s/", defaultApplicationLogPathHostRoot, md.imageName)
			md.applicationLogMount = mp
		} else {
			mountPointMap[mp.SourceVolumeID] = mp
		}
	}
	mountResult := []*MountPoint{}
	for _, mp := range mountPointMap {
		mountResult = append(mountResult, mp)
	}
	md.mountPoints = mountResult
	return nil
}

// Used to set or to update the agentCommunicationMount
func (md *ManagedDaemon) SetAgentCommunicationMount(mp *MountPoint) error {
	if mp.SourceVolumeID == defaultAgentCommunicationMount {
		mp.SourceVolumeHostPath = fmt.Sprintf("%s/%s/", defaultAgentCommunicationPathHostRoot, md.imageName)
		md.agentCommunicationMount = mp
		return nil
	} else {
		return fmt.Errorf("AgentCommunicationMount %s must have a SourceVolumeID of %s", mp.SourceVolumeID, defaultAgentCommunicationMount)
	}
}

// Used to set or to update the applicationLogMount
func (md *ManagedDaemon) SetApplicationLogMount(mp *MountPoint) error {
	if mp.SourceVolumeID == defaultApplicationLogMount {
		mp.SourceVolumeHostPath = fmt.Sprintf("%s/%s/", defaultApplicationLogPathHostRoot, md.imageName)
		md.applicationLogMount = mp
		return nil
	} else {
		return fmt.Errorf("ApplicationLogMount %s must have a SourceVolumeID of %s", mp.SourceVolumeID, defaultApplicationLogMount)
	}
}

func (md *ManagedDaemon) SetEnvironment(environment map[string]string) {
	md.environment = make(map[string]string)
	for key, val := range environment {
		md.environment[key] = val
	}
}

func (md *ManagedDaemon) SetPrivileged(isPrivileged bool) {
	md.privileged = isPrivileged
}

func (md *ManagedDaemon) GetContainerId() string {
	return md.containerId
}

func (md *ManagedDaemon) SetContainerId(containerId string) {
	md.containerId = containerId
}

func (md *ManagedDaemon) GetContainerCGroup() string {
	return md.containerCGroup
}

func (md *ManagedDaemon) SetContainerCGroup(containerCGroup string) {
	md.containerCGroup = containerCGroup
}

func (md *ManagedDaemon) GetNetworkNameSpace() string {
	return md.networkNameSpace
}

func (md *ManagedDaemon) SetNetworkNameSpace(networkNameSpace string) {
	md.networkNameSpace = networkNameSpace
}

// AddMountPoint will add by MountPoint.SourceVolume
// which is unique to the task and is a required field
// and will throw an error if an existing
// MountPoint.SourceVolume is found
func (md *ManagedDaemon) AddMountPoint(mp *MountPoint) error {
	mountIndex := md.GetMountPointIndex(mp)
	if mountIndex != -1 {
		return fmt.Errorf("MountPoint already exists at index %d", mountIndex)
	}
	md.mountPoints = append(md.mountPoints, mp)
	return nil
}

// UpdateMountPoint will update by
// MountPoint.SourceVolume which is unique to the task
// and will throw an error if the MountPoint.SourceVolume
// is not found
func (md *ManagedDaemon) UpdateMountPointBySourceVolume(mp *MountPoint) error {
	mountIndex := md.GetMountPointIndex(mp)
	if mountIndex == -1 {
		return fmt.Errorf("MountPoint %s not found; will not update", mp.SourceVolume)
	}
	md.mountPoints[mountIndex] = mp
	return nil
}

// UpdateMountPoint will delete by
// MountPoint.SourceVolume which is unique to the task
// and will throw an error if the MountPoint.SourceVolume
// is not found
func (md *ManagedDaemon) DeleteMountPoint(mp *MountPoint) error {
	mountIndex := md.GetMountPointIndex(mp)
	if mountIndex == -1 {
		return fmt.Errorf("MountPoint %s not found; will not delete", mp.SourceVolume)
	}
	md.mountPoints = append(md.mountPoints[:mountIndex], md.mountPoints[mountIndex+1:]...)
	return nil
}

// GetMountPointIndex will return index of a mountpoint or -1
// search by the unique MountPoint.SourceVolume field
func (md *ManagedDaemon) GetMountPointIndex(mp *MountPoint) int {
	sourceVolume := mp.SourceVolume
	for i, mount := range md.mountPoints {
		if mount.SourceVolume == sourceVolume {
			return i
		}
	}
	return -1
}

// AddEnvVar will add by envKey
// and will throw an error if an existing
// envKey is found
func (md *ManagedDaemon) AddEnvVar(envKey string, envVal string) error {
	_, exists := md.environment[envKey]
	if !exists {
		md.environment[envKey] = envVal
		return nil
	}
	return fmt.Errorf("EnvKey: %s already exists; will not add EnvVal: %s", envKey, envVal)
}

// Updates environment varable by evnKey
// and will throw an error if the envKey
// is not found
func (md *ManagedDaemon) UpdateEnvVar(envKey string, envVal string) error {
	_, ok := md.environment[envKey]
	if !ok {
		return fmt.Errorf("EnvKey: %s not found; will not update EnvVal: %s", envKey, envVal)
	}
	md.environment[envKey] = envVal
	return nil
}

// Deletes environment variable by envKey
// and will throw an error if the envKey
// is not found
func (md *ManagedDaemon) DeleteEnvVar(envKey string) error {
	_, ok := md.environment[envKey]
	if !ok {
		return fmt.Errorf("EnvKey: %s not found; will not delete", envKey)
	}
	delete(md.environment, envKey)
	return nil
}

// Generates a DockerHealthConfig object from the
// ManagedDaeemon Health Check fields
func (md *ManagedDaemon) GetDockerHealthConfig() *dockercontainer.HealthConfig {
	return &dockercontainer.HealthConfig{
		Test:     md.healthCheckTest,
		Interval: md.healthCheckInterval,
		Timeout:  md.healthCheckTimeout,
		Retries:  md.healthCheckRetries,
	}
}

// Validates that all required fields are present and valid
func (md *ManagedDaemon) IsValidManagedDaemon() bool {
	isValid := true
	isValid = isValid && (md.agentCommunicationMount != nil)
	isValid = isValid && (md.applicationLogMount != nil)
	return isValid
}
