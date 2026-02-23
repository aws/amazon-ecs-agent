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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverCores(t *testing.T) {
	t.Run("no devices", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create empty sysfs directory
		sysfsPath := filepath.Join(tmpDir, "sys/class/neuron_device")
		require.NoError(t, os.MkdirAll(sysfsPath, 0755))
		
		cores, err := DiscoverCores(tmpDir)
		assert.NoError(t, err)
		assert.Nil(t, cores)
	})

	t.Run("single device with multiple cores", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create sysfs structure
		sysfsPath := filepath.Join(tmpDir, "sys/class/neuron_device/neuron0")
		require.NoError(t, os.MkdirAll(sysfsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sysfsPath, "core_count"), []byte("2"), 0644))
		
		// Create device file
		devPath := filepath.Join(tmpDir, "dev")
		require.NoError(t, os.MkdirAll(devPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(devPath, "neuron0"), []byte{}, 0644))
		
		cores, err := DiscoverCores(tmpDir)
		require.NoError(t, err)
		
		expected := &NeuronCores{
			cores: map[string]NeuronCore{
				"0": {ID: "0", Device: NeuronDevice{ID: "0", Path: "/dev/neuron0"}},
				"1": {ID: "1", Device: NeuronDevice{ID: "0", Path: "/dev/neuron0"}},
			},
		}
		assert.Equal(t, expected, cores)
	})

	t.Run("multiple devices", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create device 0 with 2 cores
		sysfsPath0 := filepath.Join(tmpDir, "sys/class/neuron_device/neuron0")
		require.NoError(t, os.MkdirAll(sysfsPath0, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sysfsPath0, "core_count"), []byte("2"), 0644))
		
		// Create device 1 with 2 cores
		sysfsPath1 := filepath.Join(tmpDir, "sys/class/neuron_device/neuron1")
		require.NoError(t, os.MkdirAll(sysfsPath1, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sysfsPath1, "core_count"), []byte("2"), 0644))
		
		// Create device files
		devPath := filepath.Join(tmpDir, "dev")
		require.NoError(t, os.MkdirAll(devPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(devPath, "neuron0"), []byte{}, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(devPath, "neuron1"), []byte{}, 0644))
		
		cores, err := DiscoverCores(tmpDir)
		require.NoError(t, err)
		
		expected := &NeuronCores{
			cores: map[string]NeuronCore{
				"0": {ID: "0", Device: NeuronDevice{ID: "0", Path: "/dev/neuron0"}},
				"1": {ID: "1", Device: NeuronDevice{ID: "0", Path: "/dev/neuron0"}},
				"2": {ID: "2", Device: NeuronDevice{ID: "1", Path: "/dev/neuron1"}},
				"3": {ID: "3", Device: NeuronDevice{ID: "1", Path: "/dev/neuron1"}},
			},
		}
		assert.Equal(t, expected, cores)
	})

	t.Run("missing device file", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		sysfsPath := filepath.Join(tmpDir, "sys/class/neuron_device/neuron0")
		require.NoError(t, os.MkdirAll(sysfsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sysfsPath, "core_count"), []byte("2"), 0644))
		
		// Don't create device file
		devPath := filepath.Join(tmpDir, "dev")
		require.NoError(t, os.MkdirAll(devPath, 0755))
		
		cores, err := DiscoverCores(tmpDir)
		assert.Error(t, err)
		assert.Nil(t, cores)
		assert.Contains(t, err.Error(), "device file /dev/neuron0 not found")
	})

	t.Run("invalid core count", func(t *testing.T) {
		testCases := []struct {
			name          string
			coreCount     string
			errorContains string
		}{
			{
				name:          "non-numeric",
				coreCount:     "invalid",
				errorContains: "invalid core_count",
			},
			{
				name:          "negative cores",
				coreCount:     "-1",
				errorContains: "negative core_count",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tmpDir := t.TempDir()
				
				sysfsPath := filepath.Join(tmpDir, "sys/class/neuron_device/neuron0")
				require.NoError(t, os.MkdirAll(sysfsPath, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(sysfsPath, "core_count"), []byte(tc.coreCount), 0644))
				
				devPath := filepath.Join(tmpDir, "dev")
				require.NoError(t, os.MkdirAll(devPath, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(devPath, "neuron0"), []byte{}, 0644))
				
				cores, err := DiscoverCores(tmpDir)
				assert.Error(t, err)
				assert.Nil(t, cores)
				assert.Contains(t, err.Error(), tc.errorContains)
			})
		}
	})

	t.Run("zero cores", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		sysfsPath := filepath.Join(tmpDir, "sys/class/neuron_device/neuron0")
		require.NoError(t, os.MkdirAll(sysfsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sysfsPath, "core_count"), []byte("0"), 0644))
		
		devPath := filepath.Join(tmpDir, "dev")
		require.NoError(t, os.MkdirAll(devPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(devPath, "neuron0"), []byte{}, 0644))
		
		cores, err := DiscoverCores(tmpDir)
		assert.NoError(t, err)
		assert.Nil(t, cores)
	})
}

func TestGetUniqueDevices(t *testing.T) {
	device0 := NeuronDevice{ID: "0", Path: "/dev/neuron0"}
	device1 := NeuronDevice{ID: "1", Path: "/dev/neuron1"}
	
	cores := []NeuronCore{
		{ID: "0", Device: device0},
		{ID: "1", Device: device0},
		{ID: "2", Device: device1},
		{ID: "3", Device: device1},
	}
	
	devices := GetUniqueDevices(cores)
	assert.ElementsMatch(t, []NeuronDevice{device0, device1}, devices)
}

func TestGetCoreIDs(t *testing.T) {
	device := NeuronDevice{ID: "0", Path: "/dev/neuron0"}
	cores := []NeuronCore{
		{ID: "0", Device: device},
		{ID: "1", Device: device},
		{ID: "2", Device: device},
	}
	
	coreIDs := GetCoreIDs(cores)
	assert.Equal(t, []string{"0", "1", "2"}, coreIDs)
}

func TestCoreIDs(t *testing.T) {
	t.Run("nil cores", func(t *testing.T) {
		var nc *NeuronCores
		assert.Nil(t, nc.CoreIDs())
	})
	
	t.Run("empty cores", func(t *testing.T) {
		nc := &NeuronCores{cores: make(map[string]NeuronCore)}
		assert.Nil(t, nc.CoreIDs())
	})
	
	t.Run("with cores", func(t *testing.T) {
		device := NeuronDevice{ID: "0", Path: "/dev/neuron0"}
		nc := &NeuronCores{
			cores: map[string]NeuronCore{
				"0": {ID: "0", Device: device},
				"1": {ID: "1", Device: device},
			},
		}
		coreIDs := nc.CoreIDs()
		assert.ElementsMatch(t, []string{"0", "1"}, coreIDs)
	})
}
