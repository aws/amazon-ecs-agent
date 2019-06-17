// +build unit

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
package efs

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const (
	testIP              = "0.0.0.0"
	testTargetDirectory = "target"
	testSourceDirectory = "source"
	testNamespacePath   = "/test/ns/path"
	testNsHandle        = netns.NsHandle(1)
)

// TestMount_HappyCase sets all of the values in NFSMount and asserts that each function is called with the expected
// values.
func TestMount_HappyCase(t *testing.T) {
	defer reset()
	mountSyscall = func(device string, target string, fstype string, flags uintptr, opts string) error {
		assert.Equal(t, device, "0.0.0.0:/source")
		assert.Equal(t, target, testTargetDirectory)
		assert.Equal(t, fstype, "nfs")
		assert.Equal(t, flags, uintptr(0))
		assert.Equal(t, opts, "rsize=1048576,wsize=1048576,timeo=10,hard,retrans=2,noresvport,vers=4,addr=0.0.0.0")
		return nil
	}

	getNamespaceHelper = func(nspath string) (netns.NsHandle, error) {
		assert.Equal(t, nspath, "/test/ns/path")
		return testNsHandle, nil
	}

	setNamespaceSyscall = func(handle netns.NsHandle) error {
		assert.Equal(t, netns.NsHandle(1), handle)
		return nil
	}

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
		IPAddress:       testIP,
		NamespacePath:   testNamespacePath,
		SourceDirectory: testSourceDirectory,
	}

	assert.NoError(t, mount.Mount())
}

func TestMount_NoIPAddressError(t *testing.T) {
	defer reset()
	mountSyscall = mountFunction(nil)

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
	}

	assert.Error(t, mount.Mount())
}

func TestMount_MountError(t *testing.T) {
	defer reset()
	expectedErr := errors.New("generic mount error")
	mountSyscall = mountFunction(expectedErr)

	mount := &NFSMount{
		IPAddress:       testIP,
		TargetDirectory: testTargetDirectory,
	}

	assert.EqualError(t, mount.Mount(), expectedErr.Error())
}

func TestUnmount_HappyCase(t *testing.T) {
	defer reset()
	unmountSyscall = func(target string, flags int) error {
		assert.Equal(t, testTargetDirectory, target)
		assert.Equal(t, flags, 0)
		return nil
	}

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
	}

	assert.NoError(t, mount.Unmount())
}

func TestUnmount_ErrorCase(t *testing.T) {
	defer reset()
	expectedErr := errors.New("generic unmount error")
	unmountSyscall = unmountFunction(expectedErr)

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
	}

	assert.EqualError(t, mount.Unmount(), expectedErr.Error())
}

func TestMount_BadNamespace(t *testing.T) {
	defer reset()
	mountSyscall = mountFunction(nil)
	expectedErr := errors.New("generic namespace get error")
	getNamespaceHelper = getNamespaceHelperFunction(testNsHandle, expectedErr)

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
		IPAddress:       testIP,
		NamespacePath:   testNamespacePath,
	}

	assert.EqualError(t, mount.Mount(), expectedErr.Error())
}

func TestMount_SetNamespaceFails(t *testing.T) {
	defer reset()
	mountSyscall = mountFunction(nil)
	expectedErr := errors.New("generic namespace set error")
	getNamespaceHelper = getNamespaceHelperFunction(testNsHandle, nil)
	setNamespaceSyscall = setNamespaceFunction(expectedErr)

	mount := &NFSMount{
		TargetDirectory: testTargetDirectory,
		IPAddress:       testIP,
		NamespacePath:   testNamespacePath,
	}

	assert.EqualError(t, mount.Mount(), expectedErr.Error())
}

// The following functions are helpers for making simple mocks

func mountFunction(err error) func(string, string, string, uintptr, string) error {
	return func(string, string, string, uintptr, string) error {
		return err
	}
}

func unmountFunction(err error) func(string, int) error {
	return func(string, int) error {
		return err
	}
}

func setNamespaceFunction(err error) func(netns.NsHandle) error {
	return func(netns.NsHandle) error {
		return err
	}
}

func getNamespaceHelperFunction(handle netns.NsHandle, err error) func(string) (netns.NsHandle, error) {
	return func(string) (netns.NsHandle, error) {
		return handle, err
	}
}

func reset() {
	mountSyscall = unix.Mount
	unmountSyscall = unix.Unmount
	setNamespaceSyscall = netns.Set
	getNamespaceHelper = netns.GetFromPath
}
