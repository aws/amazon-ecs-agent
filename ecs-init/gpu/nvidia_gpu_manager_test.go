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
	"fmt"
	"os"
	"os/exec"
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
	// The device loop now probes virtualization mode to gate MPS; these are not vGPUs.
	mockDevice1.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)
	mockDevice2.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)

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
	// GetGPUDeviceIDs derives each UUID (once) and probes virtualization mode.
	mockDevice1.EXPECT().GetUUID().Return("gpu-0123", nvml.SUCCESS)
	mockDevice2.EXPECT().GetUUID().Return("gpu-1234", nvml.SUCCESS)
	// The device loop now probes virtualization mode to gate MPS; these are not vGPUs.
	mockDevice1.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)
	mockDevice2.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)
	// DetectGPUMemory then looks each discovered UUID back up and reads NVML v2
	// memory (usable = Total - Reserved).
	// reserved -> 16384 - 512 = 15872 MiB.
	mockDevice1.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{
		Total:    16384 * bytesPerMiB,
		Reserved: 512 * bytesPerMiB,
	}, nvml.SUCCESS)
	mockDevice2.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{
		Total:    16384 * bytesPerMiB,
		Reserved: 512 * bytesPerMiB,
	}, nvml.SUCCESS)

	// GetGPUDeviceIDs enumerates by index; DetectGPUMemory resolves handles by UUID.
	oldDeviceGetHandleByIndex := nvml.DeviceGetHandleByIndex
	nvml.DeviceGetHandleByIndex = func(idx int) (nvml.Device, nvml.Return) {
		if idx == 0 {
			return mockDevice1, nvml.SUCCESS
		}
		return mockDevice2, nvml.SUCCESS
	}
	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(uuid string) (nvml.Device, nvml.Return) {
		if uuid == "gpu-0123" {
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

	// Stub the host probes so Setup records deterministic MPS facts.
	oldStatFile := statFile
	statFile = func(string) (os.FileInfo, error) { return nil, nil } // binary present
	restoreExec := stubExec("enabled\n")                             // service enabled

	defer func() {
		MatchFilePattern = FilePatternMatch
		InitializeNVML = InitNVML
		NvmlGetDriverVersion = GetNvidiaDriverVersion
		NvmlGetDeviceCount = GetDeviceCount
		nvml.DeviceGetHandleByIndex = oldDeviceGetHandleByIndex
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
		WriteContentToFile = WriteToFile
		ShutdownNVML = ShutdownNVMLib
		statFile = oldStatFile
		restoreExec()
	}()

	err := nvidiaGPUManager.Setup()
	assert.NoError(t, err)
	assert.Equal(t, driverVersion, nvidiaGPUManager.(*NvidiaGPUManager).DriverVersion)
	assert.Equal(t, []string{"gpu-0123", "gpu-1234"}, nvidiaGPUManager.(*NvidiaGPUManager).GPUIDs)
	// Setup must record per-GPU usable memory (v2 Total - Reserved) per UUID.
	assert.Equal(t, map[string]uint64{"gpu-0123": 15872, "gpu-1234": 15872}, nvidiaGPUManager.(*NvidiaGPUManager).GPUMemoryMiB)
	// Setup must persist the MPS gating facts gathered from the host and devices.
	assert.True(t, nvidiaGPUManager.(*NvidiaGPUManager).MpsControlBinaryPresent)
	assert.True(t, nvidiaGPUManager.(*NvidiaGPUManager).MpsServiceEnabled)
	assert.False(t, nvidiaGPUManager.(*NvidiaGPUManager).HasVGPU)
}

// TestDetectGPUMemory covers the happy path: every discovered GPU reports v2
// memory, so a full per-UUID map is returned with no error.
func TestDetectGPUMemory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager().(*NvidiaGPUManager)
	// Memory detection iterates the UUIDs already discovered by GetGPUDeviceIDs.
	nvidiaGPUManager.GPUIDs = []string{"gpu-0", "gpu-1"}

	dev0 := mock_gpu.NewMockGPUDevice(ctrl)
	dev0.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{
		Total:    24576 * bytesPerMiB,
		Reserved: 768 * bytesPerMiB,
	}, nvml.SUCCESS)
	dev1 := mock_gpu.NewMockGPUDevice(ctrl)
	dev1.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{
		Total:    16384 * bytesPerMiB,
		Reserved: 512 * bytesPerMiB,
	}, nvml.SUCCESS)

	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(uuid string) (nvml.Device, nvml.Return) {
		if uuid == "gpu-0" {
			return dev0, nvml.SUCCESS
		}
		return dev1, nvml.SUCCESS
	}
	defer func() {
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
	}()

	memory, err := nvidiaGPUManager.DetectGPUMemory()
	assert.NoError(t, err)
	assert.Equal(t, map[string]uint64{"gpu-0": 23808, "gpu-1": 15872}, memory)
}

// TestDetectGPUMemoryHandleError verifies memory detection is all-or-none: a
// device whose handle cannot be resolved by UUID fails the whole pass with an
// error and no partial map.
func TestDetectGPUMemoryHandleError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager().(*NvidiaGPUManager)
	nvidiaGPUManager.GPUIDs = []string{"gpu-nohandle"}

	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(string) (nvml.Device, nvml.Return) {
		return nil, nvml.ERROR_NOT_FOUND
	}
	defer func() {
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
	}()

	memory, err := nvidiaGPUManager.DetectGPUMemory()
	assert.Error(t, err)
	assert.Nil(t, memory)
}

// TestDetectGPUMemoryReadError verifies an NVML v2 memory read failure fails the
// whole pass (all-or-none) rather than skipping that device.
func TestDetectGPUMemoryReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager().(*NvidiaGPUManager)
	nvidiaGPUManager.GPUIDs = []string{"gpu-memerr"}

	memErr := mock_gpu.NewMockGPUDevice(ctrl)
	memErr.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{}, nvml.ERROR_UNKNOWN)

	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(string) (nvml.Device, nvml.Return) {
		return memErr, nvml.SUCCESS
	}
	defer func() {
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
	}()

	memory, err := nvidiaGPUManager.DetectGPUMemory()
	assert.Error(t, err)
	assert.Nil(t, memory)
}

// TestDetectGPUMemoryBadData verifies a total < reserved reading is treated as a
// hard error (all-or-none), not silently skipped.
func TestDetectGPUMemoryBadData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager().(*NvidiaGPUManager)
	nvidiaGPUManager.GPUIDs = []string{"gpu-baddata"}

	badData := mock_gpu.NewMockGPUDevice(ctrl)
	badData.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{
		Total:    512 * bytesPerMiB,
		Reserved: 1024 * bytesPerMiB,
	}, nvml.SUCCESS)

	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(string) (nvml.Device, nvml.Return) {
		return badData, nvml.SUCCESS
	}
	defer func() {
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
	}()

	memory, err := nvidiaGPUManager.DetectGPUMemory()
	assert.Error(t, err)
	assert.Nil(t, memory)
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

// TestSetupMemoryError verifies Setup fails (all-or-none) when a GPU's memory
// cannot be read, so the instance does not register with partial memory data.
func TestSetupMemoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nvidiaGPUManager := NewNvidiaGPUManager()

	MatchFilePattern = func(string) ([]string, error) {
		return []string{"/dev/nvidia0"}, nil
	}
	InitializeNVML = func() error { return nil }
	NvmlGetDriverVersion = func() (string, error) { return "396.44", nil }
	NvmlGetDeviceCount = func() (int, error) { return 1, nil }

	mockDevice := mock_gpu.NewMockGPUDevice(ctrl)
	mockDevice.EXPECT().GetUUID().Return("gpu-0123", nvml.SUCCESS)
	mockDevice.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)
	// The v2 memory read fails, which must fail Setup rather than register partial data.
	mockDevice.EXPECT().GetMemoryInfo_v2().Return(nvml.Memory_v2{}, nvml.ERROR_UNKNOWN)

	oldDeviceGetHandleByIndex := nvml.DeviceGetHandleByIndex
	nvml.DeviceGetHandleByIndex = func(int) (nvml.Device, nvml.Return) {
		return mockDevice, nvml.SUCCESS
	}
	oldDeviceGetHandleByUUID := nvml.DeviceGetHandleByUUID
	nvml.DeviceGetHandleByUUID = func(string) (nvml.Device, nvml.Return) {
		return mockDevice, nvml.SUCCESS
	}
	ShutdownNVML = func() error { return nil }

	defer func() {
		MatchFilePattern = FilePatternMatch
		InitializeNVML = InitNVML
		NvmlGetDriverVersion = GetNvidiaDriverVersion
		NvmlGetDeviceCount = GetDeviceCount
		nvml.DeviceGetHandleByIndex = oldDeviceGetHandleByIndex
		nvml.DeviceGetHandleByUUID = oldDeviceGetHandleByUUID
		ShutdownNVML = ShutdownNVMLib
	}()

	err := nvidiaGPUManager.Setup()
	assert.Error(t, err)
}

// stubExec makes execCommand return a process whose stdout is the given text.
// It re-execs the test binary running TestHelperProcess, the standard Go idiom
// for faking exec.Command output without a real systemctl.
func stubExec(output string) func() {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", output}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
	return func() { execCommand = orig }
}

// TestHelperProcess is not a real test; it is the fake subprocess stubExec spawns.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Last arg is the stdout to emit.
	args := os.Args
	fmt.Fprint(os.Stdout, args[len(args)-1])
	os.Exit(0)
}

func TestDetectMPSControlBinary(t *testing.T) {
	orig := statFile
	defer func() { statFile = orig }()

	statFile = func(string) (os.FileInfo, error) { return nil, nil }
	assert.True(t, detectMpsControlBinary(), "binary present -> true")

	statFile = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	assert.False(t, detectMpsControlBinary(), "binary absent -> false")
}

func TestDetectMpsServiceEnabled(t *testing.T) {
	cases := []struct {
		out  string
		want bool
	}{
		{"enabled\n", true},
		{"enabled-runtime\n", true},
		{"disabled\n", false},
		{"static\n", false},
		{"", false}, // unit not found: no output
	}
	for _, tc := range cases {
		t.Run(tc.out, func(t *testing.T) {
			restore := stubExec(tc.out)
			defer restore()
			assert.Equal(t, tc.want, detectMpsServiceEnabled())
		})
	}
}

func TestDetectVGPU(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// A regular passthrough/bare-metal GPU is not a vGPU.
	notVGPU := mock_gpu.NewMockGPUDevice(ctrl)
	notVGPU.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS)
	assert.False(t, detectVGPU(notVGPU))

	// A vGPU slice must be detected so MPS is gated off.
	isVGPU := mock_gpu.NewMockGPUDevice(ctrl)
	isVGPU.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_VGPU, nvml.SUCCESS)
	assert.True(t, detectVGPU(isVGPU))

	// An NVML error must fail open (false) so a flaky probe never strips MPS.
	errDevice := mock_gpu.NewMockGPUDevice(ctrl)
	errDevice.EXPECT().GetVirtualizationMode().Return(nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.ERROR_UNKNOWN)
	assert.False(t, detectVGPU(errDevice))
}
