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
	asmsecretres "github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	cgroupres "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	efsres "github.com/aws/amazon-ecs-agent/agent/taskresource/efs"
	ssmsecretres "github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
)

const (
	// CgroupKey is the string used in resources map to represent cgroup resource
	CgroupKey = "cgroup"
	// DockerVolumeKey is the string used in resources map to represent docker volume
	DockerVolumeKey = "dockerVolume"
	// ASMAuthKey is the string used in resources map to represent asm auth
	ASMAuthKey = asmauthres.ResourceName
	// SSMSecretKey is the string used in resources map to represent ssm secret
	SSMSecretKey = ssmsecretres.ResourceName
	// ASMSecretKey is the string used in resources map to represent asm secret
	ASMSecretKey = asmsecretres.ResourceName
	// EFSKey is the string used in resources map to represent efs resource
	EFSKey = efsres.ResourceName
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
		if err := unmarshalResource(key, value, result); err != nil {
			return err
		}
	}
	*rm = result
	return nil
}

func unmarshalResource(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	switch key {
	case CgroupKey:
		return unmarshalCgroup(key, value, result)
	case DockerVolumeKey:
		return unmarshalDockerVolume(key, value, result)
	case ASMAuthKey:
		return unmarshalASMAuthKey(key, value, result)
	case SSMSecretKey:
		return unmarshalSSMSecretKey(key, value, result)
	case ASMSecretKey:
		return unmarshalASMSecretKey(key, value, result)
	case EFSKey:
		return unmarshalEFS(key, value, result)
	default:
		return errors.New("Unsupported resource type")
	}
}

func unmarshalCgroup(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
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

func unmarshalSSMSecretKey(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var ssmsecrets []json.RawMessage
	err := json.Unmarshal(value, &ssmsecrets)
	if err != nil {
		return err
	}

	for _, secret := range ssmsecrets {
		res := &ssmsecretres.SSMSecretResource{}
		err := res.UnmarshalJSON(secret)
		if err != nil {
			return err
		}
		result[key] = append(result[key], res)
	}
	return nil
}

func unmarshalASMSecretKey(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var asmsecrets []json.RawMessage
	err := json.Unmarshal(value, &asmsecrets)
	if err != nil {
		return err
	}

	for _, secret := range asmsecrets {
		res := &asmsecretres.ASMSecretResource{}
		err := res.UnmarshalJSON(secret)
		if err != nil {
			return err
		}
		result[key] = append(result[key], res)
	}
	return nil
}

func unmarshalEFS(key string, value json.RawMessage, result map[string][]taskresource.TaskResource) error {
	var efsSlices []json.RawMessage
	err := json.Unmarshal(value, &efsSlices)
	if err != nil {
		return err
	}
	for _, e := range efsSlices {
		efs := &efsres.EFSResource{}
		err := efs.UnmarshalJSON(e)
		if err != nil {
			return err
		}
		result[key] = append(result[key], efs)
	}
	return nil
}
