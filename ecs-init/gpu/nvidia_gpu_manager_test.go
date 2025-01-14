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

package gpu

import (
	"errors"
	"os"
	"testing"

	mock_gpu "github.com/aws/amazon-ecs-agent/ecs-init/gpu/mocks"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/golang/mock/gomock"
	_ "github.com/golang/mock/mockgen/model"
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
	NvmlGetDeviceCount = func() (int, error) {
		return 1, nil
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
	}()
	count, err := NvmlGetDeviceCount()
	assert.Equal(t, int(1), count)
	assert.NoError(t, err)
}

func TestDeviceCountError(t *testing.T) {
	NvmlGetDeviceCount = func() (int, error) {
		return 0, errors.New("device count error")
	}
	defer func() {
		NvmlGetDeviceCount = GetDeviceCount
	}()
	_, err := NvmlGetDeviceCount()
	assert.Error(t, err)
}

func TestGetGPUDeviceIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager()

	// Mock NvmlGetDeviceCount
	oldNvmlGetDeviceCount := NvmlGetDeviceCount
	NvmlGetDeviceCount = func() (int, error) {
		return 2, nil
	}
	defer func() {
		NvmlGetDeviceCount = oldNvmlGetDeviceCount
	}()

	// Mock DeviceGetHandleByIndex and DeviceGetUUID
	oldDeviceGetHandleByIndex := nvml.DeviceGetHandleByIndex
	oldDeviceGetUUID := nvml.DeviceGetUUID

	mockDevice1 := mock_gpu.NewMockGPUDevice(ctrl)
	mockDevice2 := mock_gpu.NewMockGPUDevice(ctrl)

	nvml.DeviceGetHandleByIndex = func(idx int) (nvml.Device, nvml.Return) {
		if idx == 0 {
			return mockDevice1, nvml.SUCCESS
		}
		return mockDevice2, nvml.SUCCESS
	}

	mockDevice1.EXPECT().GetUUID().Return("gpu-0123", nvml.SUCCESS)
	mockDevice2.EXPECT().GetUUID().Return("gpu-1234", nvml.SUCCESS)

	defer func() {
		nvml.DeviceGetHandleByIndex = oldDeviceGetHandleByIndex
		nvml.DeviceGetUUID = oldDeviceGetUUID
	}()

	// Call the function and assert
	gpuIDs, err := nvidiaGPUManager.GetGPUDeviceIDs()
	assert.NoError(t, err)
	assert.Equal(t, []string{"gpu-0123", "gpu-1234"}, gpuIDs)
}

func TestGetGPUDeviceIDsCountError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager()

	// Mock NvmlGetDeviceCount
	oldNvmlGetDeviceCount := NvmlGetDeviceCount
	NvmlGetDeviceCount = func() (int, error) {
		return 0, errors.New("device count error")
	}
	defer func() {
		NvmlGetDeviceCount = oldNvmlGetDeviceCount
	}()

	// Call the function and assert
	gpuIDs, err := nvidiaGPUManager.GetGPUDeviceIDs()
	assert.Error(t, err)
	assert.Empty(t, gpuIDs)
}

func TestGetGPUDeviceIDsDeviceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager()

	// Mock NvmlGetDeviceCount
	oldNvmlGetDeviceCount := NvmlGetDeviceCount
	NvmlGetDeviceCount = func() (int, error) {
		return 1, nil
	}
	defer func() {
		NvmlGetDeviceCount = oldNvmlGetDeviceCount
	}()

	// Mock DeviceGetHandleByIndex to return an error
	oldDeviceGetHandleByIndex := nvml.DeviceGetHandleByIndex
	nvml.DeviceGetHandleByIndex = func(int) (nvml.Device, nvml.Return) {
		return nil, nvml.ERROR_UNKNOWN
	}
	defer func() {
		nvml.DeviceGetHandleByIndex = oldDeviceGetHandleByIndex
	}()

	// Call the function and assert
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	NvmlGetDeviceCount = func() (int, error) {
		return 2, nil
	}

	mockDevice1 := mock_gpu.NewMockGPUDevice(ctrl)
	mockDevice2 := mock_gpu.NewMockGPUDevice(ctrl)
	mockDevice1.EXPECT().GetUUID().Return("gpu-0123", nvml.SUCCESS)
	mockDevice2.EXPECT().GetUUID().Return("gpu-1234", nvml.SUCCESS)

	// Mock DeviceGetHandleByIndex
	oldDeviceGetHandleByIndex := nvml.DeviceGetHandleByIndex
	nvml.DeviceGetHandleByIndex = func(idx int) (nvml.Device, nvml.Return) {
		if idx == 0 {
			return mockDevice1, nvml.SUCCESS
		}
		return mockDevice2, nvml.SUCCESS
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
		nvml.DeviceGetHandleByIndex = oldDeviceGetHandleByIndex
		WriteContentToFile = WriteToFile
		ShutdownNVML = ShutdownNVMLib
	}()

	err := nvidiaGPUManager.Setup()
	assert.NoError(t, err)
	assert.Equal(t, driverVersion, nvidiaGPUManager.(*NvidiaGPUManager).DriverVersion)
	assert.Equal(t, []string{"gpu-0123", "gpu-1234"}, nvidiaGPUManager.(*NvidiaGPUManager).GPUIDs)
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
