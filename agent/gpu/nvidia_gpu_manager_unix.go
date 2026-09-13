//go:build linux
// +build linux

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
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// GPUManager encompasses methods to get information on GPUs and their driver
type GPUManager interface {
	Initialize() error
	SetGPUIDs([]string)
	GetGPUIDsUnsafe() []string
	GetGPUMemoryMiBUnsafe() map[string]uint64
	SetDevices()
	GetDevices() []types.PlatformDevice
	SetDriverVersion(string)
	GetDriverVersion() string
	GetMpsControlBinaryPresent() bool
	GetMpsServiceEnabled() bool
	GetHasVGPU() bool
}

// NvidiaGPUManager is used as a wrapper for NVML APIs and implements GPUManager
// interface
type NvidiaGPUManager struct {
	DriverVersion           string                 `json:"DriverVersion"`
	GPUIDs                  []string               `json:"GPUIDs"`
	GPUDevices              []types.PlatformDevice `json:"-"`
	GPUMemoryMiB            map[string]uint64      `json:"GPUMemoryMiB,omitempty"`
	MpsControlBinaryPresent bool                   `json:"MpsControlBinaryPresent"`
	MpsServiceEnabled       bool                   `json:"MpsServiceEnabled"`
	HasVGPU                 bool                   `json:"HasVGPU"`
	lock                    sync.RWMutex
}

const (
	// GPUInfoDirPath is the directory where gpus and driver info are saved
	GPUInfoDirPath = "/var/lib/ecs/gpu"
	// NvidiaGPUInfoFilePath is the file path where gpus and driver info are saved
	NvidiaGPUInfoFilePath = GPUInfoDirPath + "/nvidia-gpu-info.json"
)

// NewNvidiaGPUManager is used to obtain NvidiaGPUManager handle
func NewNvidiaGPUManager() GPUManager {
	return &NvidiaGPUManager{}
}

// Initialize sets the fields of Nvidia GPU Manager struct
func (n *NvidiaGPUManager) Initialize() error {
	if GPUInfoFileExists() {
		// GPU info file found
		gpuJSON, err := GetGPUInfoJSON()
		if err != nil {
			return errors.Wrapf(err, "could not read GPU file content")
		}
		var nvidiaGPUInfo NvidiaGPUManager
		err = json.Unmarshal(gpuJSON, &nvidiaGPUInfo)
		if err != nil {
			return errors.Wrapf(err, "could not unmarshal GPU file content")
		}
		n.SetDriverVersion(nvidiaGPUInfo.GetDriverVersion())
		nvidiaGPUInfo.lock.RLock()
		gpuIDs := nvidiaGPUInfo.GetGPUIDsUnsafe()
		gpuMemoryMiB := nvidiaGPUInfo.GetGPUMemoryMiBUnsafe()
		nvidiaGPUInfo.lock.RUnlock()
		n.SetGPUIDs(gpuIDs)
		n.SetGPUMemoryMiB(gpuMemoryMiB)
		n.SetDevices()
		n.MpsControlBinaryPresent = nvidiaGPUInfo.GetMpsControlBinaryPresent()
		n.MpsServiceEnabled = nvidiaGPUInfo.GetMpsServiceEnabled()
		n.HasVGPU = nvidiaGPUInfo.GetHasVGPU()
	} else {
		seelog.Error("Config for GPU support is enabled, but GPU information is not found; continuing without it")
	}
	return nil
}

var GPUInfoFileExists = CheckForGPUInfoFile

func CheckForGPUInfoFile() bool {
	_, err := os.Stat(NvidiaGPUInfoFilePath)
	return !os.IsNotExist(err)
}

var GetGPUInfoJSON = GetGPUInfo

func GetGPUInfo() ([]byte, error) {
	gpuInfo, err := os.Open(NvidiaGPUInfoFilePath)
	if err != nil {
		return nil, err
	}
	defer gpuInfo.Close()

	gpuJSON, err := ioutil.ReadAll(gpuInfo)
	if err != nil {
		return nil, err
	}
	return gpuJSON, nil
}

// SetGPUIDs sets the GPUIDs
func (n *NvidiaGPUManager) SetGPUIDs(gpuIDs []string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.GPUIDs = gpuIDs
}

// GetGPUIDs returns the GPUIDs
func (n *NvidiaGPUManager) GetGPUIDsUnsafe() []string {
	return n.GPUIDs
}

// SetGPUMemoryMiB sets the per-GPU usable memory map
func (n *NvidiaGPUManager) SetGPUMemoryMiB(gpuMemoryMiB map[string]uint64) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.GPUMemoryMiB = gpuMemoryMiB
}

// GetGPUMemoryMiBUnsafe returns the per-GPU usable memory map
func (n *NvidiaGPUManager) GetGPUMemoryMiBUnsafe() map[string]uint64 {
	return n.GPUMemoryMiB
}

// SetDriverVersion is a setter for nvidia driver version
func (n *NvidiaGPUManager) SetDriverVersion(version string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.DriverVersion = version
}

// GetDriverVersion is a getter for nvidia driver version
func (n *NvidiaGPUManager) GetDriverVersion() string {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.DriverVersion
}

func (n *NvidiaGPUManager) SetDevices() {
	n.lock.Lock()
	defer n.lock.Unlock()
	gpuIDs := n.GetGPUIDsUnsafe()
	devices := make([]types.PlatformDevice, 0)
	for _, gpuID := range gpuIDs {
		device := types.PlatformDevice{
			Id:   aws.String(gpuID),
			Type: types.PlatformDeviceTypeGpu,
		}
		if memMiB, ok := n.GPUMemoryMiB[gpuID]; ok {
			device.GpuInfo = &types.GpuPlatformDeviceInfo{
				MemoryInMiB: aws.Int32(int32(memMiB)),
			}
		}
		devices = append(devices, device)
	}
	n.GPUDevices = devices
}

// GetDevices returns the GPU devices as PlatformDevices
func (n *NvidiaGPUManager) GetDevices() []types.PlatformDevice {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.GPUDevices
}

// GetMpsControlBinaryPresent reports whether ecs-init found the MPS control binary on the host.
func (n *NvidiaGPUManager) GetMpsControlBinaryPresent() bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.MpsControlBinaryPresent
}

// GetMpsServiceEnabled reports whether ecs-init found nvidia-mps.service enabled on the host.
func (n *NvidiaGPUManager) GetMpsServiceEnabled() bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.MpsServiceEnabled
}

// GetHasVGPU reports whether any device on the instance is an NVIDIA vGPU slice.
func (n *NvidiaGPUManager) GetHasVGPU() bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.HasVGPU
}
