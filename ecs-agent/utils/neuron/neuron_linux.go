//go:build linux

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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DiscoverCores enumerates Neuron devices from sysfs, validates device paths exist,
// calculates global core IDs, and returns an immutable NeuronCores structure.
// Returns nil if no devices are found (not an error).
// Returns error only for I/O failures.
// rootPath allows testing with custom sysfs location (use "/" for production).
func DiscoverCores(rootPath string) (*NeuronCores, error) {
	sysfsPath := filepath.Join(rootPath, "sys/class/neuron_device")
	devPath := filepath.Join(rootPath, "dev")

	// Enumerate devices from sysfs
	devicePaths, err := filepath.Glob(filepath.Join(sysfsPath, "neuron*"))
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate neuron devices: %w", err)
	}

	if len(devicePaths) == 0 {
		return nil, nil // No devices found - not an error
	}

	sort.Strings(devicePaths) // Ensure consistent ordering

	cores := make(map[string]NeuronCore)
	globalCoreID := 0

	for _, devicePath := range devicePaths {
		deviceName := filepath.Base(devicePath)
		deviceID := strings.TrimPrefix(deviceName, "neuron")
		deviceFilePath := fmt.Sprintf("/dev/neuron%s", deviceID)

		// Validate device file exists
		if _, err := os.Stat(filepath.Join(devPath, fmt.Sprintf("neuron%s", deviceID))); err != nil {
			return nil, fmt.Errorf("device file %s not found: %w", deviceFilePath, err)
		}

		// Read core count for this device
		coreCountPath := filepath.Join(devicePath, "core_count")
		data, err := os.ReadFile(coreCountPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read core_count for %s: %w", deviceName, err)
		}

		coreCount, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("invalid core_count for %s: %w", deviceName, err)
		}
		if coreCount < 0 {
			return nil, fmt.Errorf("negative core_count %d for %s", coreCount, deviceName)
		}

		// Create device
		device := NeuronDevice{
			ID:   deviceID,
			Path: deviceFilePath,
		}

		// Create cores for this device
		for i := 0; i < coreCount; i++ {
			core := NeuronCore{
				ID:     strconv.Itoa(globalCoreID),
				Device: device,
			}
			cores[core.ID] = core
			globalCoreID++
		}
	}

	if len(cores) == 0 {
		return nil, nil
	}

	return &NeuronCores{cores: cores}, nil
}
