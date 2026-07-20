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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

//go:generate mockgen.sh $GOPACKAGE $GOFILE

// GPUManager encompasses methods to get information on GPUs and their driver
type GPUManager interface {
	Setup() error
	Initialize() error
	Shutdown() error
	GetGPUDeviceIDs() ([]string, error)
	GetDriverVersion() (string, error)
	DetectGPUDevices() error
	SaveGPUState() error
}

// NvidiaGPUManager is used as a wrapper for NVML APIs and implements GPUManager
// interface
type NvidiaGPUManager struct {
	DriverVersion string
	GPUIDs        []string
	// MPS gating facts. The agent reads these from the saved JSON to decide whether
	// to advertise the gpu-sharing-mps capability at registration.
	MpsControlBinaryPresent bool `json:"MpsControlBinaryPresent"`
	MpsServiceEnabled       bool `json:"MpsServiceEnabled"`
	HasVGPU                 bool `json:"HasVGPU"`
}

const (
	// NvidiaGPUDeviceFilePattern is the pattern of GPU device files on the instance
	NvidiaGPUDeviceFilePattern = "/dev/nvidia*"
	// GPUInfoDirPath is the directory where gpus and driver info are saved
	GPUInfoDirPath = "/var/lib/ecs/gpu"
	// NvidiaGPUInfoFilePath is the file path where gpus and driver info are saved
	NvidiaGPUInfoFilePath = GPUInfoDirPath + "/nvidia-gpu-info.json"
	// FilePerm is the file permissions for gpu info json file
	FilePerm = 0700
	// mpsControlBinaryPath is where the NVIDIA MPS control daemon binary lives on the GPU AMI.
	mpsControlBinaryPath = "/usr/bin/nvidia-cuda-mps-control"
	// mpsServiceName is the systemd unit that runs the MPS control daemon.
	mpsServiceName = "nvidia-mps.service"
	// nvidiaEULAAgreementInfo is the EULA agreement that we want to show to the customers when using
	// Nvidia products
	nvidiaEULAAgreementInfo = "By using the GPU Optimized AMI, you agree to Nvidia’s End User License Agreement: " +
		"https://www.nvidia.com/en-us/about-nvidia/eula-agreement/"
)

// ErrNoGPUDeviceFound is thrown when it is not a ECS GPU instance
var ErrNoGPUDeviceFound = errors.New("No GPU device files found on the instance")

// NewNvidiaGPUManager is used to obtain NvidiaGPUManager handle
func NewNvidiaGPUManager() GPUManager {
	return &NvidiaGPUManager{}
}

// Setup is used for setting up gpu information in the instance
func (n *NvidiaGPUManager) Setup() error {
	seelog.Info(nvidiaEULAAgreementInfo)

	err := n.DetectGPUDevices()
	if err != nil {
		if err == ErrNoGPUDeviceFound {
			return nil
		}
		return errors.Wrapf(err, "setup failed")
	}
	err = n.Initialize()
	if err != nil {
		return errors.Wrapf(err, "setup failed")
	}
	defer n.Shutdown()
	version, err := n.GetDriverVersion()
	if err != nil {
		return errors.Wrapf(err, "setup failed")
	}
	n.DriverVersion = version
	gpuIDs, err := n.GetGPUDeviceIDs()
	if err != nil {
		return errors.Wrapf(err, "setup failed")
	}
	n.GPUIDs = gpuIDs
	// Gather the MPS facts once, after devices are known and before we persist state.
	// HasVGPU is set per-device inside GetGPUDeviceIDs above.
	n.MpsControlBinaryPresent = detectMpsControlBinary()
	n.MpsServiceEnabled = detectMpsServiceEnabled()
	err = n.SaveGPUState()
	if err != nil {
		return errors.Wrapf(err, "setup failed")
	}
	return nil
}

// DetectGPUDevices checks if GPU devices are present in the instance
func (n *NvidiaGPUManager) DetectGPUDevices() error {
	matches, err := MatchFilePattern(NvidiaGPUDeviceFilePattern)
	if err != nil {
		return errors.Wrapf(err, "detecting GPU devices failed")
	}
	if matches == nil {
		return ErrNoGPUDeviceFound
	}
	return nil
}

var MatchFilePattern = FilePatternMatch

func FilePatternMatch(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// Initialize is for initlializing nvidia's nvml library
func (n *NvidiaGPUManager) Initialize() error {
	err := InitializeNVML()
	if err != nil {
		return errors.Wrapf(err, "error initializing nvidia nvml")
	}
	return nil
}

var InitializeNVML = InitNVML

func InitNVML() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return errors.New(nvml.ErrorString(ret))
	}
	return nil
}

// Shutdown is for shutting down nvidia's nvml library
func (n *NvidiaGPUManager) Shutdown() error {
	err := ShutdownNVML()
	if err != nil {
		return errors.Wrapf(err, "error shutting down nvidia nvml")
	}
	return nil
}

var ShutdownNVML = ShutdownNVMLib

func ShutdownNVMLib() error {
	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		return errors.New(nvml.ErrorString(ret))
	}
	return nil
}

// GetDriverVersion is for getting Nvidia driver version on the instance
func (n *NvidiaGPUManager) GetDriverVersion() (string, error) {
	version, err := NvmlGetDriverVersion()
	if err != nil {
		return "", errors.Wrapf(err, "error getting nvidia driver version")
	}
	return version, err
}

var NvmlGetDriverVersion = GetNvidiaDriverVersion

func GetNvidiaDriverVersion() (string, error) {
	version, ret := nvml.SystemGetDriverVersion()
	if ret != nvml.SUCCESS {
		return "", errors.New(nvml.ErrorString(ret))
	}
	return version, nil
}

// GetGPUDeviceIDs is for getting the GPU device UUIDs
func (n *NvidiaGPUManager) GetGPUDeviceIDs() ([]string, error) {
	count, err := NvmlGetDeviceCount()
	if err != nil {
		return nil, errors.Wrapf(err, "error getting GPU device count for UUID detection")
	}
	var gpuIDs []string
	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			seelog.Errorf("Error initializing device of index %d: %v", i, nvml.ErrorString(ret))
			continue
		}
		uuid, ret := nvml.DeviceGetUUID(device)
		if ret != nvml.SUCCESS {
			seelog.Errorf("Failed to get UUID for device at index %d: %v", i, nvml.ErrorString(ret))
			continue
		}
		if detectVGPU(device) {
			// Any vGPU slice on the instance disqualifies MPS
			n.HasVGPU = true
		}
		gpuIDs = append(gpuIDs, uuid)
	}
	if len(gpuIDs) == 0 {
		return gpuIDs, errors.New("error initializing GPU devices")
	}
	return gpuIDs, nil
}

var NvmlGetDeviceCount = GetDeviceCount

// GetDeviceCount is for getting the number of GPU devices in the instance
func GetDeviceCount() (int, error) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, errors.New(nvml.ErrorString(ret))
	}
	return count, nil
}

// statFile and execCommand are indirections so unit tests can stub host access.
var statFile = os.Stat
var execCommand = exec.Command

// detectMpsControlBinary reports whether the MPS control daemon binary is installed.
func detectMpsControlBinary() bool {
	_, err := statFile(mpsControlBinaryPath)
	return err == nil
}

// detectMpsServiceEnabled reports whether nvidia-mps.service is enabled to start on boot.
// systemctl is-enabled exits non-zero for a known-but-disabled unit while still printing
// its state, so we key off the printed word rather than the exit code.
func detectMpsServiceEnabled() bool {
	out, err := execCommand("systemctl", "is-enabled", mpsServiceName).Output()
	state := strings.TrimSpace(string(out))
	if err != nil {
		seelog.Debugf("Checking nvidia-mps.service enablement: 'systemctl is-enabled %s' returned state %q, err: %v", mpsServiceName, state, err)
	}
	return state == "enabled" || state == "enabled-runtime"
}

// detectVGPU reports whether a device is running as an NVIDIA vGPU slice. It fails open:
// only a definite VGPU result counts, so an NVML error or unknown mode leaves MPS enabled.
func detectVGPU(device nvml.Device) bool {
	mode, ret := nvml.DeviceGetVirtualizationMode(device)
	if ret != nvml.SUCCESS {
		return false
	}
	return mode == nvml.GPU_VIRTUALIZATION_MODE_VGPU
}

// SaveGPUState saves gpu state info on the disk
func (n *NvidiaGPUManager) SaveGPUState() error {
	gpuManagerJSON, err := json.Marshal(n)
	if err != nil {
		return errors.Wrapf(err, "gpu info state save failed")
	}
	err = WriteContentToFile(NvidiaGPUInfoFilePath, gpuManagerJSON, FilePerm)
	if err != nil {
		return errors.Wrapf(err, "gpu info state save failed")
	}
	return nil
}

var WriteContentToFile = WriteToFile

func WriteToFile(filename string, data []byte, perm os.FileMode) error {
	err := os.MkdirAll(GPUInfoDirPath, os.ModeDir|perm)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, perm)
}
