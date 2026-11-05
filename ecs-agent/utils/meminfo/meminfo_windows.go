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
//
// This file is derived from github.com/docker/docker/pkg/meminfo (v25.0.6),
// Copyright 2013-2018 Docker, Inc., licensed under the Apache License 2.0.
// moby v29 does not publish pkg/meminfo as an importable module, so the agent
// carries its own copy.

package meminfo

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa366589(v=vs.85).aspx
// https://msdn.microsoft.com/en-us/library/windows/desktop/aa366770(v=vs.85).aspx
type memorystatusex struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

// readMemInfo retrieves memory statistics of the host system and returns a
// Memory type.
func readMemInfo() (*Memory, error) {
	msi := &memorystatusex{
		dwLength: 64,
	}
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(msi)))
	if r1 == 0 {
		return &Memory{}, nil
	}
	return &Memory{
		MemTotal:  int64(msi.ullTotalPhys),
		MemFree:   int64(msi.ullAvailPhys),
		SwapTotal: int64(msi.ullTotalPageFile),
		SwapFree:  int64(msi.ullAvailPageFile),
	}, nil
}
