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

package gpuInit

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"github.com/stretchr/testify/assert"
)

func TestNVMLInitialize(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	InitializeNVML = func() error {
		return nil
	}
	defer func() {
		InitializeNVML = InitNVML
	}()
	err := nvidiaGPUManager.Initialize()
	assert.NoError(t, err)
}

func TestNVMLInitializeError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	InitializeNVML = func() error {
		return errors.New("error initializing nvml")
	}
	defer func() {
		InitializeNVML = InitNVML
	}()
	err := nvidiaGPUManager.Initialize()
	assert.Error(t, err)
}

func TestDeviceCount(t *testing.T) {
	NvmlGetDeviceCount = func() (uint, error) {
		return 1, nil
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
	}()
	count, err := NvmlGetDeviceCount()
	assert.Equal(t, uint(1), count)
	assert.NoError(t, err)
}

func TestDeviceCountError(t *testing.T) {
	NvmlGetDeviceCount = func() (uint, error) {
		return 0, errors.New("device count error")
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
	}()
	_, err := NvmlGetDeviceCount()
	assert.Error(t, err)
}

func TestNewDeviceLite(t *testing.T) {
	model := "Tesla-k80"
	NvmlNewDeviceLite = func(idx uint) (*nvml.Device, error) {
		return &nvml.Device{
			UUID:  "gpu-0123",
			Model: &model,
		}, nil
	}
	defer func() {
		NvmlNewDeviceLite = NewDeviceLite
	}()
	device, err := NvmlNewDeviceLite(4)
	assert.NoError(t, err)
	assert.Equal(t, "gpu-0123", device.UUID)
	assert.Equal(t, model, *device.Model)
}

func TestNewDeviceLiteError(t *testing.T) {
	NvmlNewDeviceLite = func(idx uint) (*nvml.Device, error) {
		return nil, errors.New("device error")
	}
	defer func() {
		NvmlNewDeviceLite = NewDeviceLite
	}()
	device, err := NvmlNewDeviceLite(4)
	assert.Error(t, err)
	assert.Nil(t, device)
}

func TestGetGPUDeviceIDs(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	NvmlGetDeviceCount = func() (uint, error) {
		return 2, nil
	}
	NvmlNewDeviceLite = func(idx uint) (*nvml.Device, error) {
		var uuid string
		if idx == 0 {
			uuid = "gpu-0123"
		} else {
			uuid = "gpu-1234"
		}
		return &nvml.Device{
			UUID: uuid,
		}, nil
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
		NvmlNewDeviceLite = NewDeviceLite
	}()
	gpuIDs, err := nvidiaGPUManager.GetGPUDeviceIDs()
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual([]string{"gpu-0123", "gpu-1234"}, gpuIDs))
}

func TestGetGPUDeviceIDsCountError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	NvmlGetDeviceCount = func() (uint, error) {
		return 0, errors.New("device count error")
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
	}()
	gpuIDs, err := nvidiaGPUManager.GetGPUDeviceIDs()
	assert.Error(t, err)
	assert.Empty(t, gpuIDs)
}

func TestGetGPUDeviceIDsDeviceError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	NvmlGetDeviceCount = func() (uint, error) {
		return 1, nil
	}
	NvmlNewDeviceLite = func(idx uint) (*nvml.Device, error) {
		return nil, errors.New("device error")
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
		NvmlNewDeviceLite = NewDeviceLite
	}()
	gpuIDs, err := nvidiaGPUManager.GetGPUDeviceIDs()
	assert.Error(t, err)
	assert.Empty(t, gpuIDs)
}

func TestNVMLShutdown(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	ShutdownNVML = func() error {
		return nil
	}
	defer func() {
		ShutdownNVML = ShutdownNVMLib
	}()
	err := nvidiaGPUManager.Shutdown()
	assert.NoError(t, err)
}

func TestNVMLShutdownError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	ShutdownNVML = func() error {
		return errors.New("error shutting down nvml")
	}
	defer func() {
		ShutdownNVML = ShutdownNVMLib
	}()
	err := nvidiaGPUManager.Shutdown()
	assert.Error(t, err)
}

func TestNVMLDriverVersion(t *testing.T) {
	driverVersion := "396.44"
	nvidiaGPUManager := NewNvidiaGPUManager()
	NvmlGetDriverVersion = func() (string, error) {
		return driverVersion, nil
	}
	defer func() {
		NvmlGetDriverVersion = GetNvidiaDriverVersion
	}()
	version, err := nvidiaGPUManager.GetDriverVersion()
	assert.NoError(t, err)
	assert.Equal(t, driverVersion, version)
}

func TestNVMLDriverVersionError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	NvmlGetDriverVersion = func() (string, error) {
		return "", errors.New("error getting version")
	}
	defer func() {
		NvmlGetDriverVersion = GetNvidiaDriverVersion
	}()
	_, err := nvidiaGPUManager.GetDriverVersion()
	assert.Error(t, err)
}

func TestGPUDetection(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(string) ([]string, error) {
		return []string{"/dev/nvidia0", "/dev/nvidia1"}, nil
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
	}()
	err := nvidiaGPUManager.DetectGPUDevices()
	assert.NoError(t, err)
}

func TestGPUDetectionFailure(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(pattern string) ([]string, error) {
		return nil, errors.New("gpu failure")
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
	}()
	err := nvidiaGPUManager.DetectGPUDevices()
	assert.Error(t, err)
}

func TestGPUDetectionNotFound(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(pattern string) ([]string, error) {
		return nil, nil
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
	}()
	err := nvidiaGPUManager.DetectGPUDevices()
	assert.Equal(t, err, ErrNoGPUDeviceFound)
}

func TestSaveGPUState(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	nvidiaGPUManager.(*NvidiaGPUManager).DriverVersion = "396.44"
	WriteContentToFile = func(string, []byte, os.FileMode) error {
		return nil
	}
	defer func() {
		WriteContentToFile = WriteToFile
	}()
	err := nvidiaGPUManager.SaveGPUState()
	assert.NoError(t, err)
}

func TestSaveGPUStateError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	nvidiaGPUManager.(*NvidiaGPUManager).DriverVersion = "396.44"
	WriteContentToFile = func(string, []byte, os.FileMode) error {
		return errors.New("cannot write to disk")
	}
	defer func() {
		WriteContentToFile = WriteToFile
	}()
	err := nvidiaGPUManager.SaveGPUState()
	assert.Error(t, err)
}

func TestSetupNoGPU(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(pattern string) ([]string, error) {
		return nil, nil
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
	}()
	err := nvidiaGPUManager.Setup()
	assert.NoError(t, err)
}

func TestGPUSetupSuccessful(t *testing.T) {
	driverVersion := "396.44"
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(string) ([]string, error) {
		return []string{"/dev/nvidia0", "/dev/nvidia1"}, nil
	}
	InitializeNVML = func() error {
		return nil
	}
	NvmlGetDriverVersion = func() (string, error) {
		return driverVersion, nil
	}
	NvmlGetDeviceCount = func() (uint, error) {
		return 2, nil
	}
	NvmlNewDeviceLite = func(idx uint) (*nvml.Device, error) {
		var uuid string
		if idx == 0 {
			uuid = "gpu-0123"
		} else {
			uuid = "gpu-1234"
		}
		return &nvml.Device{
			UUID: uuid,
		}, nil
	}
	WriteContentToFile = func(string, []byte, os.FileMode) error {
		return nil
	}
	ShutdownNVML = func() error {
		return nil
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
		InitializeNVML = InitNVML
		NvmlGetDriverVersion = GetNvidiaDriverVersion
		NvmlGetDeviceCount = GetDeviceCount
		NvmlNewDeviceLite = NewDeviceLite
		WriteContentToFile = WriteToFile
		ShutdownNVML = ShutdownNVMLib
	}()
	err := nvidiaGPUManager.Setup()
	assert.NoError(t, err)
	assert.Equal(t, driverVersion, nvidiaGPUManager.(*NvidiaGPUManager).DriverVersion)
	assert.True(t, reflect.DeepEqual([]string{"gpu-0123", "gpu-1234"}, nvidiaGPUManager.(*NvidiaGPUManager).GPUIDs))
}

func TestSetupNVMLError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	MatchFilePattern = func(pattern string) ([]string, error) {
		return []string{"/dev/nvidia0", "/dev/nvidia1"}, nil
	}
	InitializeNVML = func() error {
		return errors.New("error initializing nvml")
	}
	ShutdownNVML = func() error {
		return nil
	}
	defer func() {
		MatchFilePattern = FilePatternMatch
		InitializeNVML = InitNVML
		ShutdownNVML = ShutdownNVMLib
	}()
	err := nvidiaGPUManager.Setup()
	assert.Error(t, err)
}
