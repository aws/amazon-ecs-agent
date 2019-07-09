// +build sudo

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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ipAddressEnvironmentKey     = "ECS_FTEST_EFS_IP_ADDRESS"
	namespaceFileEnvironmentKey = "ECS_FTEST_EFS_NETWORK_NAMESPACE"
	testData                    = "foo"

	// These files are known to exist on the test efs instance.
	// See misc/nfs/integ/*
	knownSource      = "efs-integration-test"
	knownFile        = "expected-file"
	knownFileContent = "expected-content"
)

// ipAddress is the known address of the efs volume (or localhost if employed
// with the namespace flag)
var ipAddress = os.Getenv(ipAddressEnvironmentKey)

// namespace is the path to a network namespace (eg, /proc/1234/ns/net)
var namespace = os.Getenv(namespaceFileEnvironmentKey)

// These tests require an EFS ip address and a network namespace handle in
// order to run. The way the integ test runs automatically is:
//
// 1. A docker container is created with inbound and outbound network disabled
//
// 2. The docker container starts an nfs server listening on localhost
//
// 3. These tests are given '127.0.0.1' as the IP and the network namespace of
// the docker container.
//
// Since the container's network mode is 'none', the _only_ way that the nfs
// mount succeeds is if the mount function executes in the namespace. This
// namespace swapping will be reused in awsvpc mode to send traffic over the
// task ENI. See misc/nfs/Dockerfile and misc/nfs/Makefile for more details.

// TestReadWrite verifies that we can write, read, and delete a file over NFS.
func TestReadWrite(t *testing.T) {
	skipIfEnvironmentNotSet(t)

	mountPoint := newTestMountPoint(t)
	defer mountPoint.cleanup()

	m := &NFSMount{
		IPAddress:       ipAddress,
		TargetDirectory: mountPoint.path,
		NamespacePath:   namespace,
	}

	require.NoError(t, m.Mount())
	defer func() {
		require.NoError(t, m.Unmount())
	}()

	f, err := ioutil.TempFile(mountPoint.path, "")
	require.NoError(t, err, "Couldn't open file for writing in mounted directory")

	_, err = f.WriteString(testData)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Read file back
	readData, err := ioutil.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, string(readData), testData)

	// Remove the file
	require.NoError(t, os.Remove(f.Name()))
}

// TestReadKnownFile verifies that a mounted volume with expected contents can
// be discovered and read
func TestReadKnownFile(t *testing.T) {
	skipIfEnvironmentNotSet(t)

	mountPoint := newTestMountPoint(t)
	defer mountPoint.cleanup()

	m := &NFSMount{
		IPAddress:       ipAddress,
		TargetDirectory: mountPoint.path,
		NamespacePath:   namespace,
	}

	require.NoError(t, m.Mount())
	defer func() {
		require.NoError(t, m.Unmount())
	}()

	data, err := ioutil.ReadFile(filepath.Join(mountPoint.path, knownSource, knownFile))
	require.NoError(t, err, "known file not found on test efs instance")
	assert.Equal(t, knownFileContent, string(data), "known file content mismatch")
}

// TestReadKnownFileWithSource does the same function as TestReadKnownFile, but also includes a source directory. This
// effectively causes the mount to happen at a specific directory within the NFS filesystem instead of the root of the
// filesystem.
func TestReadKnownFileWithSource(t *testing.T) {
	skipIfEnvironmentNotSet(t)

	mountPoint := newTestMountPoint(t)
	defer mountPoint.cleanup()

	m := &NFSMount{
		IPAddress:       ipAddress,
		TargetDirectory: mountPoint.path,
		SourceDirectory: knownSource,
		NamespacePath:   namespace,
	}

	require.NoError(t, m.Mount())
	defer func() {
		require.NoError(t, m.Unmount())
	}()

	data, err := ioutil.ReadFile(filepath.Join(mountPoint.path, knownFile))
	require.NoError(t, err, "known file not found on test efs instance")
	assert.Equal(t, knownFileContent, string(data), "known file content mismatch")
}

// testMountPoint is a helper struct that creates a temporary mount point on
// the host and cleans itself up after the test is finished.
type testMountPoint struct {
	path string
	t    *testing.T
}

// newTestMountPoint creates a tempdir to be used as the target mount.
func newTestMountPoint(t *testing.T) *testMountPoint {
	targetPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	return &testMountPoint{path: targetPath, t: t}
}

// cleanup simply removes the tempdir
func (d *testMountPoint) cleanup() {
	d.t.Log("Cleaning up directory: ", d.path)
	if err := os.Remove(d.path); err != nil {
		d.t.Log("Couldn't cleanup: ", err)
	}
}

func skipIfEnvironmentNotSet(t *testing.T) {
	if ipAddress == "" {
		t.Skip("EFS integration test requires valid ip address")
	}

	if namespace == "" {
		t.Skip("EFS integration test requires valid namespace path")
	}
}
