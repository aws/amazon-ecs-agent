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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
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
	DriverVersion       string
	NvidiaDockerVersion string
	GPUIds              []string
}

const (
	// NvidiaGPUDeviceFilePattern is the pattern of GPU device files on the instance
	NvidiaGPUDeviceFilePattern = "/dev/nvidia*"
	// GPUInfoDirPath is the directory where gpus and driver info are saved
	GPUInfoDirPath = "/var/lib/ecs/gpu"
	// NvidiaGPUInfoFilePath is the file path where gpus and driver info are saved
	NvidiaGPUInfoFilePath = "/var/lib/ecs/gpu/nvidia-gpu-info.json"
	// FilePerm is the file permissions for gpu info json file
	FilePerm = 0700
)

// ErrNoGPUDeviceFound is thrown when it is not a ECS GPU instance
var ErrNoGPUDeviceFound = errors.New("No GPU device files found in the instance")

// NewNvidiaGPUManager is used to obtain NvidiaGPUManager handle
func NewNvidiaGPUManager() GPUManager {
	return &NvidiaGPUManager{}
}

// Setup is used for setting up gpu information in the instance
func (n *NvidiaGPUManager) Setup() error {
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
	gpuIds, err := n.GetGPUDeviceIDs()
	if err != nil {
		return errors.Wrapf(err, "setup failed")
	}
	n.GPUIds = gpuIds
	err = n.SaveGPUState()
	if err != nil {
		return errors.Wrapf(err, "nvidia gpu manager: setup failed")
	}
	return nil
}

// DetectGPUDevices checks if GPU devices are present in the instance
func (n *NvidiaGPUManager) DetectGPUDevices() error {
	matches, err := MatchFilePattern(NvidiaGPUDeviceFilePattern)
	if err != nil {
		return errors.Wrapf(err, "nvidia gpu manager: detecting GPU devices failed")
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
		return errors.Wrapf(err, "nvidia gpu manager: error initializing nvidia nvml")
	}
	return nil
}

var InitializeNVML = InitNVML

func InitNVML() error {
	return nvml.Init()
}

// Shutdown is for shutting down nvidia's nvml library
func (n *NvidiaGPUManager) Shutdown() error {
	err := ShutdownNVML()
	if err != nil {
		return errors.Wrapf(err, "nvidia gpu manager: error shutting down nvidia nvml")
	}
	return nil
}

var ShutdownNVML = ShutdownNVMLib

func ShutdownNVMLib() error {
	return nvml.Shutdown()
}

// GetDriverVersion is for getting Nvidia driver version on the instance
func (n *NvidiaGPUManager) GetDriverVersion() (string, error) {
	version, err := NvmlGetDriverVersion()
	if err != nil {
		return "", errors.Wrapf(err, "nvidia gpu manager: error getting nvidia driver version")
	}
	return version, err
}

var NvmlGetDriverVersion = GetNvidiaDriverVersion

func GetNvidiaDriverVersion() (string, error) {
	return nvml.GetDriverVersion()
}

// GetGPUDeviceIDs is for getting the GPU device UUIDs
func (n *NvidiaGPUManager) GetGPUDeviceIDs() ([]string, error) {
	count, err := NvmlGetDeviceCount()
	if err != nil {
		return nil, errors.Wrapf(err, "nvidia gpu manager: error getting GPU device count for UUID detection")
	}
	var gpuIds []string
	var i uint
	for i = 0; i < count; i++ {
		device, err := NvmlNewDeviceLite(i)
		if err != nil {
			seelog.Errorf("nvidia gpu manager: error initializing device of index %d: %v", i, err)
			continue
		}
		gpuIds = append(gpuIds, device.UUID)
	}
	if len(gpuIds) == 0 {
		return gpuIds, errors.New("nvidia gpu manager: error initializing GPU devices")
	}
	return gpuIds, nil
}

var NvmlGetDeviceCount = GetDeviceCount

// GetDeviceCount is for getting the number of GPU devices in the instance
func GetDeviceCount() (uint, error) {
	return nvml.GetDeviceCount()
}

var NvmlNewDeviceLite = NewDeviceLite

// NewDeviceLite is for initializing a new GPU device
func NewDeviceLite(idx uint) (*nvml.Device, error) {
	return nvml.NewDeviceLite(idx)
}

// SaveGPUState saves gpu state info on the disk
func (n *NvidiaGPUManager) SaveGPUState() error {
	gpuManagerJSON, err := json.Marshal(n)
	if err != nil {
		return errors.Wrapf(err, "nvidia gpu manager: gpu info state save failed")
	}
	err = WriteContentToFile(NvidiaGPUInfoFilePath, gpuManagerJSON, FilePerm)
	if err != nil {
		return errors.Wrapf(err, "nvidia gpu manager: gpu info state save failed")
	}
	return nil
}

var WriteContentToFile = WriteToFile

func WriteToFile(filename string, data []byte, perm os.FileMode) error {
	err := os.MkdirAll(GPUInfoDirPath, os.ModeDir|perm)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, perm)
}
