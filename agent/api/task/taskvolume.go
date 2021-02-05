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

package task

import (
	"encoding/json"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/fsxwindowsfileserver"
	taskresourcetypes "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	HostVolumeType                 = "host"
	DockerVolumeType               = "docker"
	EFSVolumeType                  = "efs"
	FSxWindowsFileServerVolumeType = "fsxWindowsFileServer"
)

// TaskVolume is a definition of all the volumes available for containers to
// reference within a task. It must be named.
type TaskVolume struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Volume taskresourcevolume.Volume
}

// UnmarshalJSON for TaskVolume determines the name and volume type, and
// unmarshals it into the appropriate HostVolume fulfilling interfaces
func (tv *TaskVolume) UnmarshalJSON(b []byte) error {
	// Format: {name: volumeName, host: HostVolume, dockerVolumeConfiguration {}}
	intermediate := make(map[string]json.RawMessage)
	if err := json.Unmarshal(b, &intermediate); err != nil {
		return err
	}
	name, ok := intermediate["name"]
	if !ok {
		return errors.New("invalid Volume; must include a name")
	}
	if err := json.Unmarshal(name, &tv.Name); err != nil {
		return err
	}

	volumeType, ok := intermediate["type"]
	if !ok {
		volumeType = []byte(`"host"`)
		seelog.Infof("Unmarshal task volume: volume type not specified, default to host")
	}
	if err := json.Unmarshal(volumeType, &tv.Type); err != nil {
		return err
	}

	switch tv.Type {
	case HostVolumeType:
		return tv.unmarshalHostVolume(intermediate["host"])
	case DockerVolumeType:
		return tv.unmarshalDockerVolume(intermediate["dockerVolumeConfiguration"])
	case EFSVolumeType:
		return tv.unmarshalEFSVolume(intermediate["efsVolumeConfiguration"])
	case FSxWindowsFileServerVolumeType:
		return tv.unmarshalFSxWindowsFileServerVolume(intermediate["fsxWindowsFileServerVolumeConfiguration"])
	default:
		return errors.Errorf("unrecognized volume type: %q", tv.Type)
	}
}

// MarshalJSON overrides the logic for JSON-encoding a TaskVolume object
func (tv *TaskVolume) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})

	if len(tv.Type) == 0 {
		tv.Type = HostVolumeType
	}

	result["name"] = tv.Name
	result["type"] = tv.Type

	switch tv.Type {
	case DockerVolumeType:
		result["dockerVolumeConfiguration"] = tv.Volume
	case HostVolumeType:
		result["host"] = tv.Volume
	case EFSVolumeType:
		result["efsVolumeConfiguration"] = tv.Volume
	case FSxWindowsFileServerVolumeType:
		result["fsxWindowsFileServerVolumeConfiguration"] = tv.Volume
	default:
		return nil, errors.Errorf("unrecognized volume type: %q", tv.Type)
	}

	return json.Marshal(result)
}

func (tv *TaskVolume) unmarshalDockerVolume(data json.RawMessage) error {
	if data == nil {
		return errors.New("invalid volume: empty volume configuration")
	}
	var dockerVolumeConfig taskresourcevolume.DockerVolumeConfig
	err := json.Unmarshal(data, &dockerVolumeConfig)
	if err != nil {
		return err
	}

	tv.Volume = &dockerVolumeConfig
	return nil
}

func (tv *TaskVolume) unmarshalEFSVolume(data json.RawMessage) error {
	if data == nil {
		return errors.New("invalid volume: empty volume configuration")
	}
	var efsVolumeConfig taskresourcevolume.EFSVolumeConfig
	err := json.Unmarshal(data, &efsVolumeConfig)
	if err != nil {
		return err
	}

	tv.Volume = &efsVolumeConfig
	return nil
}

func (tv *TaskVolume) unmarshalFSxWindowsFileServerVolume(data json.RawMessage) error {
	if data == nil {
		return errors.New("invalid volume: empty volume configuration")
	}
	var fsxWindowsFileServerVolumeConfig fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig
	err := json.Unmarshal(data, &fsxWindowsFileServerVolumeConfig)
	if err != nil {
		return err
	}

	tv.Volume = &fsxWindowsFileServerVolumeConfig
	return nil
}

func (tv *TaskVolume) unmarshalHostVolume(data json.RawMessage) error {
	if data == nil {
		return errors.New("invalid volume: empty volume configuration")
	}

	// Default to trying to unmarshal it as a FSHostVolume
	var hostvolume taskresourcevolume.FSHostVolume
	err := json.Unmarshal(data, &hostvolume)
	if err != nil {
		return err
	}
	if hostvolume.FSSourcePath == "" {
		// If the FSSourcePath is empty, that must mean it was not an
		// FSHostVolume (empty path is invalid for that type).
		// Unmarshal it as local docker volume.
		localVolume := &taskresourcevolume.LocalDockerVolume{}
		json.Unmarshal(data, localVolume)
		tv.Volume = localVolume
	} else {
		tv.Volume = &hostvolume
	}
	return nil
}

// getEFSVolumeDriverName returns the driver name for creating the EFS volume.
func getEFSVolumeDriverName(cfg *config.Config) string {
	if taskresourcevolume.UseECSVolumePlugin(cfg) {
		return taskresourcevolume.ECSVolumePlugin
	}
	return taskresourcevolume.DockerLocalDriverName
}

// getDockerVolumeResource retrieves docker volume resource from task resource map.
func (task *Task) getDockerVolumeResource() ([]taskresource.TaskResource, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	res, ok := task.ResourcesMapUnsafe[taskresourcetypes.DockerVolumeKey]
	return res, ok
}

// SetPausePIDInVolumeResources sets the pause container pid field in each volume resource.
func (task *Task) SetPausePIDInVolumeResources(pid string) {
	resources, ok := task.getDockerVolumeResource()
	if !ok {
		return
	}

	for _, res := range resources {
		volRes := res.(*taskresourcevolume.VolumeResource)
		volRes.SetPauseContainerPID(pid)
	}
}
