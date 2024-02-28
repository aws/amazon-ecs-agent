//go:build windows
// +build windows

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

package utils

import (
	"runtime"

	"golang.org/x/sys/windows"
)

var (
	win32APIGetAllActiveProcessorCount = windows.GetActiveProcessorCount
	golangRuntimeNumCPU                = runtime.NumCPU
)

// GetNumCPU On Windows, runtime.NumCPU() does not return the correct value for vCPU > 64
// To resolve this, we are using MSFT implementation of the function: https://github.com/microsoft/hcsshim/blob/dd45838a9bf9ff8f431847aaf3e4421763c15c49/internal/processorinfo/processor_count.go#L14
// Background around issue: https://github.com/moby/moby/issues/38935, https://learn.microsoft.com/en-us/windows/win32/procthread/processor-groups
// Explanation: As was mentioned above, many of the Windows processor affinity functions will only return the information for a single Processor Group.
// Since a single group can only hold 64 logical processors, this means when there are more they will be divided into multiple groups.
// Golang runtime.NumCPU will only return the value of one Processor Group (not the sum of all).
func GetNumCPU() int {
	if amount := win32APIGetAllActiveProcessorCount(windows.ALL_PROCESSOR_GROUPS); amount != 0 {
		return int(amount)
	}
	// If the Win32 API does not return correctly, fall back to runtime.NumCPU()
	return golangRuntimeNumCPU()
}
