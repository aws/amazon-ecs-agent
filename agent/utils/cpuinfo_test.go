// +build linux,unit

// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package utils

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadCPUInfoWithFlags(t *testing.T) {
	path := filepath.Join(".", "testdata", "test_cpu_info")
	cpuInfo, err := ReadCPUInfo(path)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(cpuInfo.Processors))
	assert.Equal(t, 8, len(cpuInfo.Processors[0].Flags))
	assert.Equal(t, 10, len(cpuInfo.Processors[1].Flags))
}

func TestReadCPUInfoWithFlagsARM(t *testing.T) {
	path := filepath.Join(".", "testdata", "test_cpu_info_arm")
	cpuInfo, err := ReadCPUInfo(path)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(cpuInfo.Processors))
	assert.Equal(t, 8, len(cpuInfo.Processors[0].Flags))
	assert.Equal(t, 9, len(cpuInfo.Processors[1].Flags))
}

func TestReadCPUInfoNoFlags(t *testing.T) {
	path := filepath.Join(".", "testdata", "test_cpu_info_no_flag")
	cpuInfo, err := ReadCPUInfo(path)
	assert.Nil(t, err)
	proc := cpuInfo.Processors[0]
	assert.Nil(t, proc.Flags)
}

func TestReadCPUInfoError(t *testing.T) {
	path := filepath.Join(".", "testdata", "test_cpu_info_error")
	cpu, err := ReadCPUInfo(path)
	assert.Nil(t, cpu)
	assert.Error(t, err)
}

func TestGetCPUFlags(t *testing.T) {
	proc1 := Processor{
		Flags: []string{"flag1", "flag2"},
	}
	proc2 := Processor{
		Flags: []string{"flag1", "flag3"},
	}
	cpu := &CPUInfo{
		[]Processor{proc1, proc2},
	}

	flagMap := GetCPUFlags(cpu)
	assert.Equal(t, 3, len(flagMap))
}
