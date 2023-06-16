//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package manageddaemon

import (
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

const (
	TestImageName         = "TestImage"
	TestImageCanonicalRef = "TestRef:latest"
	TestImagePath         = "/test/image/path"
	TestAgentPath         = "/test/agent/path"
	TestMountPointVolume  = "testVolume"
)

func TestNewManagedDaemon(t *testing.T) {
	cases := []struct {
		TestName              string
		TestImageName         string
		TestAgentMount        *MountPoint
		ExpectedDaemonName    string
		ExpectedMountVolume   string
		ExpectedMountReadOnly bool
	}{
		{
			TestName:              "All Fields",
			TestImageName:         TestImageName,
			TestAgentMount:        &MountPoint{SourceVolume: TestMountPointVolume, ReadOnly: true},
			ExpectedDaemonName:    TestImageName,
			ExpectedMountVolume:   TestMountPointVolume,
			ExpectedMountReadOnly: true,
		},
		{
			TestName:              "Empty Image Name",
			TestImageName:         "",
			TestAgentMount:        &MountPoint{SourceVolume: TestMountPointVolume, ReadOnly: true},
			ExpectedDaemonName:    "",
			ExpectedMountVolume:   TestMountPointVolume,
			ExpectedMountReadOnly: true,
		},
		{
			TestName:              "Missing Mount ReadOnly Field",
			TestImageName:         TestImageName,
			TestAgentMount:        &MountPoint{SourceVolume: TestMountPointVolume},
			ExpectedDaemonName:    TestImageName,
			ExpectedMountVolume:   TestMountPointVolume,
			ExpectedMountReadOnly: false,
		},
	}

	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			tmd := NewManagedDaemon(
				c.TestImageName,
				TestImageCanonicalRef,
				TestImagePath,
				c.TestAgentMount,
				"")
			assert.Equal(t, c.ExpectedDaemonName, tmd.GetImageName(), "Wrong value for Managed Daemon Image Name")
			assert.Equal(t, c.ExpectedMountVolume, tmd.GetAgentCommunicationMount().SourceVolume, "Wrong value for Managed Daemon Mount Source Volume")
			assert.Equal(t, c.ExpectedMountReadOnly, tmd.GetAgentCommunicationMount().ReadOnly, "Wrong value for Managed Daemon ReadOnly")
			assert.Equal(t, "", tmd.GetAgentCommunicationFileName(), "Wrong value for Managed Daemon Agent Communication File Name")
		})
	}
}

func TestSetMountPoints(t *testing.T) {
	cases := []struct {
		TestName       string
		TestAgentMount *MountPoint
		TestMountCount int
	}{
		{
			TestName:       "All Fields",
			TestAgentMount: &MountPoint{SourceVolume: TestMountPointVolume, ReadOnly: true},
			TestMountCount: 0,
		},
		{
			TestName:       "Empty Image Name",
			TestAgentMount: &MountPoint{SourceVolume: TestMountPointVolume, ReadOnly: true},
			TestMountCount: 1,
		},
		{
			TestName:       "Missing Mount ReadOnly Field",
			TestAgentMount: &MountPoint{SourceVolume: TestMountPointVolume},
			TestMountCount: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			tmd := NewManagedDaemon(
				TestImageName,
				TestImageCanonicalRef,
				TestImagePath,
				c.TestAgentMount,
				"")
			mountPoints := []*MountPoint{}
			for i := 0; i < c.TestMountCount; i++ {
				mountPoints = append(mountPoints, c.TestAgentMount)
			}
			tmd.SetMountPoints(mountPoints)
			assert.Equal(t, c.TestMountCount, len(tmd.GetMountPoints()), "Wrong value for Set Managed Daemon Mounts")
		})
	}
}

func TestAddMountPoint(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetMountPoints(mountPoints)
	// test add valid mount point
	errResult := tmd.AddMountPoint(testMountPoint2)
	assert.Equal(t, 2, len(tmd.GetMountPoints()))
	assert.Equal(t, "TestMountPointVolume2", tmd.GetMountPoints()[1].SourceVolume)
	assert.Nil(t, errResult)

	// test add existing mount point -- should fail
	errResult = tmd.AddMountPoint(testMountPoint2)
	expectedErrorMsg := "MountPoint already exists at index 1"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestUpdateMountPoint(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetMountPoints(mountPoints)
	assert.Equal(t, 1, len(tmd.GetMountPoints()))
	assert.False(t, tmd.GetMountPoints()[0].ReadOnly)

	// test update existing mount point
	updatedMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1", ReadOnly: true}
	errResult := tmd.UpdateMountPointBySourceVolume(updatedMountPoint1)
	assert.Equal(t, 1, len(tmd.GetMountPoints()))
	assert.True(t, tmd.GetMountPoints()[0].ReadOnly)

	// test update non-existing mount point
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	errResult = tmd.UpdateMountPointBySourceVolume(testMountPoint2)
	expectedErrorMsg := "MountPoint TestMountPointVolume2 not found; will not update"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestDeleteMountPoint(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetMountPoints(mountPoints)
	assert.Equal(t, 1, len(tmd.GetMountPoints()))
	assert.False(t, tmd.GetMountPoints()[0].ReadOnly)

	// test delete non-existing mount point
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	errResult := tmd.DeleteMountPoint(testMountPoint2)
	assert.Equal(t, 1, len(tmd.GetMountPoints()))
	expectedErrorMsg := "MountPoint TestMountPointVolume2 not found; will not delete"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)

	// test delete existing mount point
	errResult = tmd.DeleteMountPoint(testMountPoint1)
	assert.Equal(t, 0, len(tmd.mountPoints))
}

func TestSetEnvironment(t *testing.T) {
	cases := []struct {
		TestName           string
		TestEnvironmentMap map[string]string
	}{
		{
			TestName:           "Missing Map",
			TestEnvironmentMap: nil,
		},
		{
			TestName:           "Single Element Map",
			TestEnvironmentMap: map[string]string{"testKey1": "testVal1"},
		},
		{
			TestName:           "Multi Map",
			TestEnvironmentMap: map[string]string{"testKey1": "testVal1", "TestKey2": "TestVal2"},
		},
	}

	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			tmd := NewManagedDaemon(
				TestImageName,
				TestImageCanonicalRef,
				TestImagePath,
				&MountPoint{SourceVolume: TestMountPointVolume},
				"")
			tmd.SetEnvironment(c.TestEnvironmentMap)
			assert.Equal(t, len(c.TestEnvironmentMap), len(tmd.GetEnvironment()), "Wrong value for Set Environment")
		})
	}
}

func TestAddEnvVar(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetEnvironment(map[string]string{"testKey1": "testVal1", "TestKey2": "TestVal2"})
	// test add new EnvKey
	errResult := tmd.AddEnvVar("testKey3", "testVal3")
	assert.Nil(t, errResult)
	assert.Equal(t, 3, len(tmd.GetEnvironment()))
	assert.Equal(t, "testVal3", tmd.GetEnvironment()["testKey3"])

	// test add existing EnvKey -- should fail
	errResult = tmd.AddEnvVar("testKey3", "nope")
	assert.Equal(t, 3, len(tmd.GetEnvironment()))
	assert.Equal(t, "testVal3", tmd.GetEnvironment()["testKey3"])
	expectedErrorMsg := "EnvKey: testKey3 already exists; will not add EnvVal: nope"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestUpdateEnvVar(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetEnvironment(map[string]string{"testKey1": "testVal1", "TestKey2": "TestVal2"})
	// test update EnvKey
	errResult := tmd.UpdateEnvVar("TestKey2", "TestValNew")
	assert.Nil(t, errResult)
	assert.Equal(t, 2, len(tmd.GetEnvironment()))
	assert.Equal(t, "TestValNew", tmd.GetEnvironment()["TestKey2"])

	// test update non-existing EnvKey -- should fail
	errResult = tmd.UpdateEnvVar("testKey3", "nope")
	assert.Equal(t, 2, len(tmd.GetEnvironment()))
	expectedErrorMsg := "EnvKey: testKey3 not found; will not update EnvVal: nope"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestDeleteEnvVar(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetEnvironment(map[string]string{"testKey1": "testVal1", "TestKey2": "TestVal2"})
	// test delete EnvKey
	errResult := tmd.DeleteEnvVar("TestKey2")
	assert.Nil(t, errResult)
	assert.Equal(t, 1, len(tmd.GetEnvironment()))

	// test delete non-existing EnvKey -- should fail
	errResult = tmd.DeleteEnvVar("testKey3")
	assert.Equal(t, 1, len(tmd.GetEnvironment()))
	expectedErrorMsg := "EnvKey: testKey3 not found; will not delete"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestGetDockerHealthCheckConfig(t *testing.T) {
	testHealthCheck := []string{"echo", "test"}
	testHealthInterval := 1 * time.Minute
	testHealthTimeout := 2 * time.Minute
	testHealthRetries := 5
	expectedDockerCheck := &dockercontainer.HealthConfig{
		Test:     testHealthCheck,
		Interval: testHealthInterval,
		Timeout:  testHealthTimeout,
		Retries:  testHealthRetries,
	}
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	tmd := NewManagedDaemon(TestImageName, TestImageCanonicalRef, TestImagePath, testMountPoint1, "")
	tmd.SetHealthCheck(testHealthCheck, testHealthInterval, testHealthTimeout, testHealthRetries)
	dockerCheck := tmd.GetDockerHealthConfig()
	assert.Equal(t, expectedDockerCheck, dockerCheck)
}
