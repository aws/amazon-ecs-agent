//go:build windows
// +build windows

/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// Info returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func Info(path string) (int64, int64, int64, int64, int64, int64, error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64
	var err error

	// The equivalent linux call supports calls against files but the syscall for windows
	// fails for files with error code: The directory name is invalid. (#99173)
	// https://docs.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-
	// By always ensuring the directory path we meet all uses cases of this function
	path = filepath.Dir(path)
	ret, _, err := syscall.Syscall6(
		procGetDiskFreeSpaceEx.Addr(),
		4,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(path))),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
		0,
		0,
	)
	if ret == 0 {
		return 0, 0, 0, 0, 0, 0, err
	}

	return freeBytesAvailable, totalNumberOfBytes, totalNumberOfBytes - freeBytesAvailable, 0, 0, 0, nil
}
