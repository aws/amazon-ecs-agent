// This file is derived from the moby/moby (Docker) project.
// The original code may be found at:
// https://github.com/moby/moby/blob/master/pkg/meminfo/meminfo_windows.go
//
// Copyright 2013-2018 Docker, Inc. Licensed under the Apache License, Version 2.0.
//
// Modifications are Copyright Amazon.com Inc. or its affiliates. Licensed under the Apache License 2.0.
//
// Vendored into the ECS agent because moby v29 no longer publishes pkg/meminfo
// as a supported public Go module (it lives only in the non-importable
// github.com/moby/moby/v2 root module). See .scratch/docker-moby-migration ticket 08.

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
