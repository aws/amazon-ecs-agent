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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
)

var devices = []types.PlatformDevice{
	{
		Id:   aws.String("id1"),
		Type: types.PlatformDeviceTypeGpu,
	},
	{
		Id:   aws.String("id2"),
		Type: types.PlatformDeviceTypeGpu,
	},
	{
		Id:   aws.String("id3"),
		Type: types.PlatformDeviceTypeGpu,
	},
}

func TestNvidiaGPUManagerInitialize(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return []byte(`{"DriverVersion":"396.44","GPUIDs":["id1","id2","id3"],` +
			`"MpsControlBinaryPresent":true,"MpsServiceEnabled":true,"HasVGPU":false}`), nil
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
	// The MPS gating facts written by ecs-init must round-trip through Initialize.
	assert.True(t, nvidiaGPUManager.GetMpsControlBinaryPresent())
	assert.True(t, nvidiaGPUManager.GetMpsServiceEnabled())
	assert.False(t, nvidiaGPUManager.GetHasVGPU())
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

// TestSetDevicesWithGPUMemory verifies SetDevices attaches GpuInfo.MemoryInMiB
// for UUIDs with known usable memory, and leaves GpuInfo nil for UUIDs without
// it (legacy hosts / partial NVML discovery), so the field is omitted from RCI.
func TestSetDevicesWithGPUMemory(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager().(*NvidiaGPUManager)
	nvidiaGPUManager.SetGPUIDs([]string{"id1", "id2"})
	// id1 has known memory; id2 does not.
	nvidiaGPUManager.SetGPUMemoryMiB(map[string]uint64{"id1": 15872})
	nvidiaGPUManager.SetDevices()

	got := nvidiaGPUManager.GetDevices()
	expected := []types.PlatformDevice{
		{
			Id:   aws.String("id1"),
			Type: types.PlatformDeviceTypeGpu,
			GpuInfo: &types.GpuPlatformDeviceInfo{
				MemoryInMiB: aws.Int32(15872),
			},
		},
		{
			Id:   aws.String("id2"),
			Type: types.PlatformDeviceTypeGpu,
		},
	}
	assert.True(t, reflect.DeepEqual(expected, got), "expected %+v, got %+v", expected, got)
	assert.Nil(t, got[1].GpuInfo, "GpuInfo must be nil for a UUID without known memory")
}

// TestInitializeWithGPUMemory verifies GPUMemoryMiB round-trips through the GPU
// info file and lands on the reported devices.
func TestInitializeWithGPUMemory(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return []byte(`{"DriverVersion":"396.44","GPUIDs":["id1","id2"],` +
			`"GPUMemoryMiB":{"id1":15872,"id2":15872},` +
			`"MpsControlBinaryPresent":true,"MpsServiceEnabled":true,"HasVGPU":false}`), nil
	}
	defer func() {
		GPUInfoFileExists = CheckForGPUInfoFile
		GetGPUInfoJSON = GetGPUInfo
	}()
	err := nvidiaGPUManager.Initialize()
	assert.NoError(t, err)
	for _, device := range nvidiaGPUManager.GetDevices() {
		assert.NotNil(t, device.GpuInfo, "device %s should carry GpuInfo", aws.ToString(device.Id))
		assert.Equal(t, int32(15872), aws.ToInt32(device.GpuInfo.MemoryInMiB))
	}
}
