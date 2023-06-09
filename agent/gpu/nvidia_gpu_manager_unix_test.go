//go:build linux && unit
// +build linux,unit

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

package gpu

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

var devices = []*ecs.PlatformDevice{
	{
		Id:   aws.String("id1"),
		Type: aws.String(ecs.PlatformDeviceTypeGpu),
	},
	{
		Id:   aws.String("id2"),
		Type: aws.String(ecs.PlatformDeviceTypeGpu),
	},
	{
		Id:   aws.String("id3"),
		Type: aws.String(ecs.PlatformDeviceTypeGpu),
	},
}

func TestNvidiaGPUManagerInitialize(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return []byte(`{"DriverVersion":"396.44","GPUIDs":["id1","id2","id3"]}`), nil
	}
	defer func() {
		GPUInfoFileExists = CheckForGPUInfoFile
		GetGPUInfoJSON = GetGPUInfo
	}()
	err := nvidiaGPUManager.Initialize()
	assert.NoError(t, err)
	assert.Equal(t, []string{"id1", "id2", "id3"}, nvidiaGPUManager.GetGPUIDsUnsafe())
	assert.Equal(t, "396.44", nvidiaGPUManager.GetDriverVersion())
	assert.True(t, reflect.DeepEqual(devices, nvidiaGPUManager.GetDevices()))
}

func TestNvidiaGPUManagerError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return nil, errors.New("corrupted content")
	}
	defer func() {
		GPUInfoFileExists = CheckForGPUInfoFile
		GetGPUInfoJSON = GetGPUInfo
	}()
	err := nvidiaGPUManager.Initialize()
	assert.Error(t, err)
	assert.Nil(t, nvidiaGPUManager.GetGPUIDsUnsafe())
	assert.Empty(t, nvidiaGPUManager.GetDriverVersion())
}

func TestSetGPUDevices(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	nvidiaGPUManager.SetGPUIDs([]string{"id1", "id2", "id3"})
	nvidiaGPUManager.SetDevices()
	assert.True(t, reflect.DeepEqual(devices, nvidiaGPUManager.GetDevices()))
}
