//go:build windows
// +build windows

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

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

package util

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Define functions from Windows API
var (
	// We will define them here so that they are loaded by default for Windows
	// instead of loading them on each invocation of this method.
	kernel32                              = windows.NewLazySystemDLL("kernel32.dll")
	procGetVolumeNameForVolumeMountPointW = kernel32.NewProc("GetVolumeNameForVolumeMountPointW")

	// funcGetVolumeNameForVolumeMountPointW is the GetVolumeNameForVolumeMountPointW Win32 API.
	// Reference: https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumenameforvolumemountpointw
	funcGetVolumeNameForVolumeMountPointW func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error) = procGetVolumeNameForVolumeMountPointW.Call
)

// IsBlockDevice checks if the given path is a block device on Windows.
func IsBlockDevice(fullPath string) (bool, error) {
	// Ensure the fullPath ends with a backslash.
	// This is as per the API documentation- https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumenameforvolumemountpointw#parameters
	if !strings.HasSuffix(fullPath, "\\") {
		fullPath += "\\"
	}

	var volumeName [windows.MAX_PATH + 1]uint16
	mountPointUTF16, err := windows.UTF16PtrFromString(fullPath)
	if err != nil {
		return false, fmt.Errorf("failed to convert path <%q> to UTF-16 pointer: %w", fullPath, err)
	}

	ret, _, err := funcGetVolumeNameForVolumeMountPointW(
		uintptr(unsafe.Pointer(mountPointUTF16)),
		uintptr(unsafe.Pointer(&volumeName[0])),
		windows.MAX_PATH+1,
	)
	// If the return value is zero, then it means the call failed.
	// We will unwrap the error and return the same.
	if ret == 0 {
		var errno windows.Errno
		ok := errors.As(err, &errno)
		if !ok {
			return false, fmt.Errorf("unknown error occurred while checking block device: %v", err)
		}
		return false, fmt.Errorf("error checking block device at %q: %s", fullPath, errno.Error())
	}

	return true, nil
}
