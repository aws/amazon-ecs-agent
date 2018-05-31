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
)

const (
	// CgroupKey is the string used in resources map to represent cgroup resource
	CgroupKey = "cgroup"
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
		// TODO: add a case for volume resource. Currently it is not added since it does
		// not fully implement TaskResource
		default:
			return errors.New("Unsupported resource type")
		}
	}
	*rm = result
	return nil
}
