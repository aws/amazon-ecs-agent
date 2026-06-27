/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dcgm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// DCGM_NVSDM_MOCK_YAML environment variable for enabling NVSDM mock configuration
	DCGM_NVSDM_MOCK_YAML = "DCGM_NVSDM_MOCK_YAML"
	// DCGM_DBG_FILE is environment variables which enables DCGM to write debug logs to a specific file
	DCGM_DBG_FILE = "__DCGM_DBG_FILE"
	// DCGM_DBG_LVL is environment variables which enables DCGM logging level
	DCGM_DBG_LVL = "__DCGM_DBG_LVL"
)

func setupTest(tb testing.TB) func(testing.TB) {
	// Store original debug settings
	originalDebugLevel, hasDebugLevel := os.LookupEnv(DCGM_DBG_LVL)
	originalDebugFile, hasDebugFile := os.LookupEnv(DCGM_DBG_FILE)

	// Enable debug output to stdout
	err := os.Setenv(DCGM_DBG_LVL, "6")
	require.NoError(tb, err)
	err = os.Setenv(DCGM_DBG_FILE, "/dev/stdout")
	require.NoError(tb, err)

	// Initialize DCGM
	cleanup, err := Init(Embedded)
	assert.NoError(tb, err)

	return func(tb testing.TB) {
		defer cleanup()

		// Restore original debug settings
		if hasDebugLevel {
			_ = os.Setenv(DCGM_DBG_LVL, originalDebugLevel)
		} else {
			_ = os.Unsetenv(DCGM_DBG_LVL)
		}

		if hasDebugFile {
			_ = os.Setenv(DCGM_DBG_FILE, originalDebugFile)
		} else {
			_ = os.Unsetenv(DCGM_DBG_FILE)
		}
	}
}

func runOnlyWithLiveGPUs(t *testing.T) {
	t.Helper()

	gpus, err := getSupportedDevices()
	require.NoError(t, err)

	if len(gpus) < 1 {
		t.Skip("Skipping test that requires live GPUs. None were found")
	}
}

func withInjectionGPUs(tb testing.TB, count int) ([]uint, error) {
	tb.Helper()
	numGPUs, err := GetAllDeviceCount()
	require.NoError(tb, err)

	if numGPUs+1 > MAX_NUM_DEVICES {
		tb.Skipf("Unable to add fake GPU with more than %d gpus", MAX_NUM_DEVICES)
	}

	entityList := make([]MigHierarchyInfo, count)
	for i := range entityList {
		entityList[i] = MigHierarchyInfo{
			Entity: GroupEntityPair{EntityGroupId: FE_GPU},
		}
	}

	return CreateFakeEntities(entityList)
}

// withInjectionGPUInstances creates fake GPU instances on the specified GPU.
// It returns a map of fake GPU instance IDs to their parent GPU ID.
func withInjectionGPUInstances(tb testing.TB, gpuId uint, instanceCount int) (map[uint]uint, error) {
	tb.Helper()

	if instanceCount <= 0 {
		return nil, nil
	}

	entities := make([]MigHierarchyInfo, 0, instanceCount)
	for i := 0; i < instanceCount; i++ {
		entities = append(entities, MigHierarchyInfo{
			Parent: GroupEntityPair{
				EntityGroupId: FE_GPU,
				EntityId:      gpuId,
			},
			Entity: GroupEntityPair{
				EntityGroupId: FE_GPU_I,
			},
		})
	}

	createdIDs, err := CreateFakeEntities(entities)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]uint, len(createdIDs))
	for _, id := range createdIDs {
		result[id] = gpuId
	}

	return result, nil
}

// withInjectionComputeInstances creates fake compute instances on the specified GPU instances.
// It returns a mapping of compute instance IDs to their parent GPU instance IDs.
// If count is 0 or parentIDs is empty, it returns an empty map.
func withInjectionComputeInstances(tb testing.TB, parentIDs []uint, count int) (map[uint]uint, error) {
	tb.Helper()

	if count <= 0 {
		return nil, nil
	}

	if len(parentIDs) == 0 {
		return nil, nil
	}

	entities := make([]MigHierarchyInfo, 0, count)
	instanceIndex := 0
	for i := 0; i < count; i++ {
		if instanceIndex >= len(parentIDs) {
			instanceIndex = 0
		}
		entities = append(entities, MigHierarchyInfo{
			Parent: GroupEntityPair{
				EntityGroupId: FE_GPU_I,
				EntityId:      parentIDs[instanceIndex],
			},
			Entity: GroupEntityPair{
				EntityGroupId: FE_GPU_CI,
			},
		})
		instanceIndex++
	}

	createdIDs, err := CreateFakeEntities(entities)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]uint, len(createdIDs))
	instanceIndex = 0
	for _, id := range createdIDs {
		if instanceIndex >= len(parentIDs) {
			instanceIndex = 0
		}
		result[id] = parentIDs[instanceIndex]
		instanceIndex++
	}

	return result, nil
}

// withNvsdmMockConfig runs a test with a specified NVSDM mock configuration
// It handles setting up and tearing down the environment variable for the mock config
func withNvsdmMockConfig(t *testing.T, configYamlPath string, testFunc func(t *testing.T)) {
	t.Helper()

	// Get absolute path for the config file

	absPath, err := filepath.Abs(configYamlPath)
	require.NoError(t, err, "Failed to get absolute path for config file")

	// Check if config file exists
	if _, err = os.Stat(absPath); os.IsNotExist(err) {
		t.Skipf("Skip test due to missing config YAML file [%s]", absPath)
		return
	}

	// Store original env var value if it exists
	originalValue, hasOriginal := os.LookupEnv(DCGM_NVSDM_MOCK_YAML)

	// Set the environment variable
	err = os.Setenv(DCGM_NVSDM_MOCK_YAML, absPath)
	require.NoError(t, err, "Failed to set mock config environment variable")

	// Cleanup function to restore original state
	defer func() {
		if hasOriginal {
			_ = os.Setenv(DCGM_NVSDM_MOCK_YAML, originalValue)
		} else {
			_ = os.Unsetenv(DCGM_NVSDM_MOCK_YAML)
		}
	}()

	// Run the test
	testFunc(t)
}
