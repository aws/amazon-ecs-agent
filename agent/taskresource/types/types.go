// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package types

import (
	"encoding/json"
	"errors"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	asmauthres "github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	cgroupres "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
)

const (
	// CgroupKey is the string used in resources map to represent cgroup resource
	CgroupKey = "cgroup"
	// DockerVolumeKey is the string used in resources map to represent docker volume
	DockerVolumeKey = "dockerVolume"
	ASMAuthKey      = asmauthres.ResourceName
)

// ResourcesMap represents the map of resource type to the corresponding resource
// objects
type ResourcesMap map[string][]taskresource.TaskResource

// UnmarshalJSON unmarshals ResourcesMap object
func (rm *ResourcesMap) UnmarshalJSON(data []byte) error {
	resources := make(map[string]json.RawMessage)
	err := json.Unmarshal(data, &resources)
	if err != nil {
		return err
	}
	result := make(map[string][]taskresource.TaskResource)
	for key, value := range resources {
		switch key {
		case CgroupKey:
			if unmarshlCgroup(key, value, result) != nil {
				return err
			}
		case DockerVolumeKey:
			if unmarshalDockerVolume(key, value, result) != nil {
				return err
			}
		case ASMAuthKey:
			if unmarshalASMAuthKey(key, value, result) != nil {
				return err
			}
		default:
			return errors.New("Unsupported resource type")
		}
	}
	*rm = result
	return nil
}

func unmarshlCgroup(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var cgroups []json.RawMessage
	err := json.Unmarshal(value, &cgroups)
	if err != nil {
		return err
	}
	for _, c := range cgroups {
		cgroup := &cgroupres.CgroupResource{}
		err := cgroup.UnmarshalJSON(c)
		if err != nil {
			return err
		}
		result[key] = append(result[key], cgroup)
	}
	return nil
}

func unmarshalDockerVolume(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var volumes []json.RawMessage
	err := json.Unmarshal(value, &volumes)
	if err != nil {
		return err
	}
	for _, vol := range volumes {
		dockerVolume := &volume.VolumeResource{}
		err := dockerVolume.UnmarshalJSON(vol)
		if err != nil {
			return err
		}
		result[key] = append(result[key], dockerVolume)
	}
	return nil
}

func unmarshalASMAuthKey(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var asmauths []json.RawMessage
	err := json.Unmarshal(value, &asmauths)
	if err != nil {
		return err
	}

	for _, a := range asmauths {
		auth := &asmauthres.ASMAuthResource{}
		err := auth.UnmarshalJSON(a)
		if err != nil {
			return err
		}
		result[key] = append(result[key], auth)
	}
	return nil
}
