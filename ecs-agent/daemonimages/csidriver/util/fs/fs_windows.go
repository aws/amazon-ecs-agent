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

package fs

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = modkernel32.NewProc("GetDiskFreeSpaceExW")
)

type UsageInfo struct {
	Bytes  int64
	Inodes int64
}

// Info returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func Info(path string) (
	available int64, capacity int64, used int64,
	totalInodes int64, freeInodes int64, usedInodes int64,
	err error) {
	var totalNumberOfFreeBytes int64
	// The equivalent linux call supports calls against files but the syscall for windows
	// fails for files with error code: The directory name is invalid. (#99173)
	// https://docs.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-
	// By always ensuring the directory path we meet all uses cases of this function
	path = filepath.Dir(path)
	ret, _, err := syscall.Syscall6(
		procGetDiskFreeSpaceEx.Addr(),
		4,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(path))),
		uintptr(unsafe.Pointer(&available)),
		uintptr(unsafe.Pointer(&capacity)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
		0,
		0,
	)
	if ret == 0 {
		available = 0
		capacity = 0
		used = 0

		totalInodes = 0
		freeInodes = 0
		usedInodes = 0

		return
	}

	used = capacity - available
	totalInodes = 0
	freeInodes = 0
	usedInodes = 0
	return
}
