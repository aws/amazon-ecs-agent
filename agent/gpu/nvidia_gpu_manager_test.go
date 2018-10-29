// +build linux,unit

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNvidiaGPUManagerInitialize(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return []byte(`{"DriverVersion":"396.44","NvidiaDockerVersion":"1.0","GPUIDs":["id1","id2","id3"]}`), nil
	}
	defer func() {
		GPUInfoFileExists = CheckForGPUInfoFile
		GetGPUInfoJSON = GetGPUInfo
	}()
	err := nvidiaGPUManager.Initialize()
	assert.NoError(t, err)
	assert.Equal(t, nvidiaGPUManager.GetGPUIDs(), []string{"id1", "id2", "id3"})
	assert.Equal(t, nvidiaGPUManager.GetDriverVersion(), "396.44")
	assert.Equal(t, nvidiaGPUManager.GetRuntimeVersion(), "1.0")
}

func TestNvidiaGPUManagerError(t *testing.T) {
	nvidiaGPUManager := NewNvidiaGPUManager()
	GPUInfoFileExists = func() bool {
		return true
	}
	GetGPUInfoJSON = func() ([]byte, error) {
		return nil, errors.New("corrupted content")
	}
	defer func() {
		GPUInfoFileExists = CheckForGPUInfoFile
		GetGPUInfoJSON = GetGPUInfo
	}()
	err := nvidiaGPUManager.Initialize()
	assert.Error(t, err)
	assert.Nil(t, nvidiaGPUManager.GetGPUIDs())
	assert.Empty(t, nvidiaGPUManager.GetDriverVersion())
	assert.Empty(t, nvidiaGPUManager.GetRuntimeVersion())
}
