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

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// GPUManager encompasses methods to get information on GPUs and their driver
type GPUManager interface {
	Initialize() error
	SetGPUIDs([]string)
	GetGPUIDsUnsafe() []string
	SetDevices()
	GetDevices() []*ecs.PlatformDevice
	SetDriverVersion(string)
	GetDriverVersion() string
}

// NvidiaGPUManager is used as a wrapper for NVML APIs and implements GPUManager
// interface
type NvidiaGPUManager struct {
	DriverVersion string                `json:"DriverVersion"`
	GPUIDs        []string              `json:"GPUIDs"`
	GPUDevices    []*ecs.PlatformDevice `json:"-"`
	lock          sync.RWMutex
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
		nvidiaGPUInfo.lock.RUnlock()
		n.SetGPUIDs(gpuIDs)
		n.SetDevices()
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
	devices := make([]*ecs.PlatformDevice, 0)
	for _, gpuID := range gpuIDs {
		devices = append(devices, &ecs.PlatformDevice{
			Id:   aws.String(gpuID),
			Type: aws.String(ecs.PlatformDeviceTypeGpu),
		})
	}
	n.GPUDevices = devices
}

// GetDevices returns the GPU devices as PlatformDevices
func (n *NvidiaGPUManager) GetDevices() []*ecs.PlatformDevice {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.GPUDevices
}
