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

func TestEFSMount(t *testing.T) {
	m := MountHelper{
		Device: "fs-123",
		Target: "/var/lib/ecs/volumes/123",
	}
	runMount = func([]string) error {
		return nil
	}
	defer func() {
		runMount = runMountCommand
	}()
	assert.NoError(t, m.Mount())
}

func TestEFSMountFailure(t *testing.T) {
	m := MountHelper{
		Device: "xyz",
		Target: "/var/lib/ecs/volumes/123",
	}
	runMount = func([]string) error {
		return errors.New("mount failure")
	}
	defer func() {
		runMount = runMountCommand
	}()
	assert.Error(t, m.Mount())
}

func TestEFSOptionsValidate(t *testing.T) {
	m := MountHelper{
		Device: "fs-123",
	}
	assert.Error(t, m.Validate(), "missing target field")
}

func TestEFSUnmount(t *testing.T) {
	m := MountHelper{
		Device: "fs-123",
		Target: "/var/lib/ecs/volumes/123",
	}
	lookPath = func(string) (string, error) {
		return "/mount", nil
	}
	runUnmount = func(path string, target string) error {
		assert.Equal(t, "/mount", path)
		assert.Equal(t, "/var/lib/ecs/volumes/123", target)
		return nil
	}
	defer func() {
		lookPath = getPath
		runUnmount = runUnmountCommand
	}()
	assert.NoError(t, m.Unmount())
}

func TestEFSNoUnmountBinary(t *testing.T) {
	m := MountHelper{
		Device: "fs-123",
		Target: "/var/lib/ecs/volumes/123",
	}
	lookPath = func(string) (string, error) {
		return "", errors.New("unmount not found")
	}
	defer func() {
		lookPath = getPath
	}()
	assert.Error(t, m.Unmount())
}

func TestEFSUnmountError(t *testing.T) {
	m := MountHelper{
		Device: "fs-123",
		Target: "/var/lib/ecs/volumes/123",
	}
	lookPath = func(string) (string, error) {
		return "/mount", nil
	}
	runUnmount = func(path string, target string) error {
		return errors.New("unmount failure")
	}
	defer func() {
		lookPath = getPath
		runUnmount = runUnmountCommand
	}()
	assert.Error(t, m.Unmount())
}
