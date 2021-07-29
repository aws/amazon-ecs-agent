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

package volumes

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVolumeDriverCreateHappyPath(t *testing.T) {
	e := NewECSVolumeDriver()
	req := CreateRequest{
		Name: "vol",
		Path: VolumeMountPathPrefix + "vol",
		Options: map[string]string{
			"type":   "efs",
			"o":      "tls",
			"device": "fs-123",
		},
	}
	runMount = func([]string) error {
		return nil
	}
	defer func() {
		runMount = runMountCommand
	}()
	assert.NoError(t, e.Create(&req))
	assert.Len(t, e.volumeMounts, 1)
}

func TestVolumeDriverCreateFailure(t *testing.T) {
	e := NewECSVolumeDriver()
	req := CreateRequest{
		Name: "vol",
		Path: VolumeMountPathPrefix + "vol",
		Options: map[string]string{
			"type":   "efs",
			"o":      "tls",
			"device": "fs-123",
		},
	}
	runMount = func([]string) error {
		return errors.New("cannot mount")
	}
	defer func() {
		runMount = runMountCommand
	}()
	assert.Error(t, e.Create(&req), "expected error when volume cannot be mounted")
	assert.Len(t, e.volumeMounts, 0)
}

func TestCreateVolumeExists(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := CreateRequest{
		Name: "vol",
		Path: VolumeMountPathPrefix + "vol",
		Options: map[string]string{
			"type":   "efs",
			"o":      "tls",
			"device": "fs-123",
		},
	}
	assert.Error(t, e.Create(&req))
	assert.Len(t, e.volumeMounts, 1)
}

func TestCreateVolumeMissingOption(t *testing.T) {
	e := NewECSVolumeDriver()
	req := CreateRequest{
		Name: "vol",
		Path: VolumeMountPathPrefix + "vol",
		Options: map[string]string{
			"type": "efs",
			"o":    "tls",
		},
	}
	assert.Error(t, e.Create(&req), "expected error when missing device ID")
	assert.Len(t, e.volumeMounts, 0)
}

func TestRemoveVolumeHappyPath(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := RemoveRequest{
		Name: "vol",
	}
	lookPath = func(string) (string, error) {
		return "path", nil
	}
	runUnmount = func(string, string) error {
		return nil
	}
	defer func() {
		lookPath = getPath
		runUnmount = runUnmountCommand
	}()
	assert.NoError(t, e.Remove(&req))
	assert.Len(t, e.volumeMounts, 0)
}

func TestRemoveVolumeUnmounted(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := RemoveRequest{
		Name: "vol",
	}
	lookPath = func(string) (string, error) {
		return "path", nil
	}
	runUnmount = func(string, string) error {
		return errors.New("not mounted")
	}
	defer func() {
		lookPath = getPath
		runUnmount = runUnmountCommand
	}()
	assert.NoError(t, e.Remove(&req), "expected no error when unmount failed because of not mounted")
	assert.Len(t, e.volumeMounts, 0)
}

func TestRemoveUnmountFailure(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := RemoveRequest{
		Name: "vol",
	}
	lookPath = func(string) (string, error) {
		return "path", nil
	}
	runUnmount = func(string, string) error {
		return errors.New("cannot unmount")
	}
	defer func() {
		lookPath = getPath
		runUnmount = runUnmountCommand
	}()
	assert.Error(t, e.Remove(&req), "expected error when unmount fails")
	assert.Len(t, e.volumeMounts, 1)
}

func TestRemoveUnmountNotFound(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := RemoveRequest{
		Name: "vol",
	}
	lookPath = func(string) (string, error) {
		return "", errors.New("unmount binary not found")
	}
	defer func() {
		lookPath = getPath
	}()
	assert.Error(t, e.Remove(&req), "expected error when unmount binary not found")
	assert.Len(t, e.volumeMounts, 1)
}

func TestRemoveVolumeNotPresent(t *testing.T) {
	e := NewECSVolumeDriver()
	e.volumeMounts["vol"] = &MountHelper{}
	req := RemoveRequest{
		Name: "vol1",
	}
	assert.Error(t, e.Remove(&req), "expected error when volume to remove is not found")
	assert.Len(t, e.volumeMounts, 1)
}
