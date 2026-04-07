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

package neuron

import (
	"maps"
	"slices"
)

// NeuronDevice represents a physical Neuron device (chip)
type NeuronDevice struct {
	ID   string `json:"id"`   // Logical device ID: "0", "1"
	Path string `json:"path"` // Device path: "/dev/neuron0"
}

// NeuronCore represents a logical compute unit within a Neuron device
type NeuronCore struct {
	ID     string       `json:"id"`     // Global core ID: "0", "1", "2"
	Device NeuronDevice `json:"device"` // Parent device containing this core
}

// NeuronCores contains all discovered Neuron cores
// Immutable after construction - all methods return copies
type NeuronCores struct {
	cores map[string]NeuronCore // coreID -> NeuronCore
}

// CoreByID returns a copy of the core with the given ID
// Returns false if core ID is not found
func (nc *NeuronCores) CoreByID(id string) (NeuronCore, bool) {
	core, ok := nc.cores[id]
	return core, ok
}

// CoreIDs returns all core IDs as strings for registration
func (nc *NeuronCores) CoreIDs() []string {
	if nc == nil || len(nc.cores) == 0 {
		return nil
	}
	ids := make([]string, 0, len(nc.cores))
	for id := range nc.cores {
		ids = append(ids, id)
	}
	return ids
}

// GetUniqueDevices returns deduplicated devices from a slice of cores
func GetUniqueDevices(cores []NeuronCore) []NeuronDevice {
	deviceMap := make(map[string]NeuronDevice)
	for _, core := range cores {
		deviceMap[core.Device.ID] = core.Device
	}

	devices := make([]NeuronDevice, 0, len(deviceMap))
	for _, device := range deviceMap {
		devices = append(devices, device)
	}
	return devices
}

// GetCoreIDs extracts core IDs from a slice of cores
func GetCoreIDs(cores []NeuronCore) []string {
	coreIDs := make([]string, len(cores))
	for i, core := range cores {
		coreIDs[i] = core.ID
	}
	return coreIDs
}

// NewNeuronCoresForTesting creates a NeuronCores instance for testing.
// This is exported only for use in tests.
func NewNeuronCoresForTesting(coresMap map[string]NeuronCore) *NeuronCores {
	return &NeuronCores{cores: coresMap}
}

// NeuronDevices contains all discovered Neuron devices.
type NeuronDevices struct {
	devices map[string]NeuronDevice // deviceID -> NeuronDevice
}

// DeviceByID returns the device with the given ID.
// Returns false if device ID is not found.
func (nd *NeuronDevices) DeviceByID(id string) (NeuronDevice, bool) {
	d, ok := nd.devices[id]
	return d, ok
}

// DeviceIDs returns all device IDs as strings.
func (nd *NeuronDevices) DeviceIDs() []string {
	if nd == nil {
		return nil
	}
	return slices.Collect(maps.Keys(nd.devices))
}

// NewNeuronDevicesForTesting creates a NeuronDevices instance for testing.
// This is exported only for use in tests.
func NewNeuronDevicesForTesting(devicesMap map[string]NeuronDevice) *NeuronDevices {
	return &NeuronDevices{devices: devicesMap}
}
