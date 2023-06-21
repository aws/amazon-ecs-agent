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

	dockercontainer "github.com/docker/docker/api/types/container"
)

type ManagedDaemon struct {
	imageName         string
	imageCanonicalRef string
	imagePath         string

	healthCheckTest     []string
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	healthCheckRetries  int

	// Daemons will have a primary agent <-> daemon mount
	// this may either be implictly the first mount in the MountPoints array
	// or explicitly a separate mount
	agentCommunicationMount    *MountPoint
	agentCommunicationFileName string

	mountPoints []*MountPoint
	environment map[string]string
}

func NewManagedDaemon(
	imageName string,
	imageCanonicalRef string,
	imagePath string,
	agentCommunicationMount *MountPoint,
	agentCommunicationFileName string,
) *ManagedDaemon {
	newManagedDaemon := &ManagedDaemon{
		imageName:                  imageName,
		imageCanonicalRef:          imageCanonicalRef,
		imagePath:                  imagePath,
		agentCommunicationMount:    agentCommunicationMount,
		agentCommunicationFileName: agentCommunicationFileName,
	}
	return newManagedDaemon
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

func (md *ManagedDaemon) GetImageName() string {
	return md.imageName
}

func (md *ManagedDaemon) GetImageCanonicalRef() string {
	return md.imageCanonicalRef
}

func (md *ManagedDaemon) GetImagePath() string {
	return md.imagePath
}

func (md *ManagedDaemon) GetAgentCommunicationMount() *MountPoint {
	return md.agentCommunicationMount
}

func (md *ManagedDaemon) GetAgentCommunicationFileName() string {
	return md.agentCommunicationFileName
}

func (md *ManagedDaemon) GetMountPoints() []*MountPoint {
	return md.mountPoints
}

func (md *ManagedDaemon) GetEnvironment() map[string]string {
	return md.environment
}

func (md *ManagedDaemon) SetMountPoints(mountPoints []*MountPoint) {
	md.mountPoints = make([]*MountPoint, len(mountPoints))
	copy(md.mountPoints, mountPoints)
}

func (md *ManagedDaemon) SetEnvironment(environment map[string]string) {
	md.environment = make(map[string]string)
	for key, val := range environment {
		md.environment[key] = val
	}
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
	if mountIndex < 0 {
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
	if mountIndex < 0 {
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
