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
	cgroupres "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
)

const (
	// CgroupKey is the string used in resources map to represent cgroup resource
	CgroupKey = "cgroup"
	// DockerVolumeKey is the string used in resources map to represent docker volume
	DockerVolumeKey = "dockerVolume"
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
			var cgroups []json.RawMessage
			err = json.Unmarshal(value, &cgroups)
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
		case DockerVolumeKey:
			var volumes []json.RawMessage
			err = json.Unmarshal(value, &volumes)
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

		default:
			return errors.New("Unsupported resource type")
		}
	}
	*rm = result
	return nil
}
