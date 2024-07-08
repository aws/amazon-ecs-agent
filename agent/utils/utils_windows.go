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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func GetCanonicalPath(path string) string {
	lowercasedPath := strings.ToLower(path)
	// if the path is a bare drive like "d:", don't filepath.Clean it because it will add a '.'.
	// this is to fix the case where mounting from D:\ to D: is supported by docker but not ecs
	if isBareDrive(lowercasedPath) {
		return lowercasedPath
	}

	if isNamedPipesPath(lowercasedPath) {
		return lowercasedPath
	}

	return filepath.Clean(lowercasedPath)
}

func isBareDrive(path string) bool {
	if filepath.VolumeName(path) == path {
		return true
	}

	return false
}

func isNamedPipesPath(path string) bool {
	matched, err := regexp.MatchString(`\\{2}\.[\\]pipe[\\].+`, path)

	if err != nil {
		return false
	}

	return matched
}

// findUnusedDriveLetter is used to search for an available drive letter on the container instance.
// Reference: https://golang.org/src/os/os_windows_test.go
func FindUnusedDriveLetter() (string, error) {
	// Do not use A: and B:, because they are reserved for floppy drive.
	// Do not use C:, because it is normally used for main drive.
	for l := 'Z'; l >= 'D'; l-- {
		p := string(l) + `:\`
		if IsAvailableDriveLetter(p) {
			return p, nil
		}
	}
	return "", errors.New("could not find an available drive letter to mount fsxwindowsfileserver resource on the container instance")
}

var DriveLetterAvailable = IsAvailableDriveLetter

func IsAvailableDriveLetter(hostPath string) bool {
	_, err := os.Stat(hostPath)
	if os.IsNotExist(err) {
		return true
	}
	return false
}

func MkdirAllAndChown(path string, perm fs.FileMode, uid, gid int) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, perm)
	}
	if err != nil {
		return fmt.Errorf("failed to mkdir %s: %+v", path, err)
	}
	// ToDo: Fix this. Commenting for now as Chown does not work for Windows
	//if err = os.Chown(path, uid, gid); err != nil {
	//	return fmt.Errorf("failed to chown %s: %+v", path, err)
	//}
	return nil
}
