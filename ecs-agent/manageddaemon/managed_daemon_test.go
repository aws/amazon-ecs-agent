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
	"fmt"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestNewManagedDaemon(t *testing.T) {
	cases := []struct {
		testName       string
		testImageName  string
		testImageTag   string
		expectedDaemon *ManagedDaemon
	}{
		{
			testName:       "All Fields",
			testImageName:  TestImageName,
			testImageTag:   TestImageTag,
			expectedDaemon: &ManagedDaemon{imageName: TestImageName, imageTag: TestImageTag},
		},
		{
			testName:       "Missing Image Name",
			testImageName:  "",
			testImageTag:   TestImageTag,
			expectedDaemon: &ManagedDaemon{imageName: "", imageTag: TestImageTag},
		},
		{
			testName:       "Missing Image Tag",
			testImageName:  TestImageName,
			testImageTag:   "",
			expectedDaemon: &ManagedDaemon{imageName: TestImageName, imageTag: ""},
		},
	}

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			assert.Equal(t, c.expectedDaemon.GetImageName(), c.testImageName, "Wrong value for Managed Daemon Image Name")
			assert.Equal(t, c.expectedDaemon.GetImageTag(), c.testImageTag, "Wrong value for Managed Daemon Image Tag")
		})
	}
}

func TestSetMountPoints(t *testing.T) {
	cases := []struct {
		TestName           string
		TestAgentMount     *MountPoint
		TestMountCount     int
		ExpectedMountCount int
	}{
		{
			TestName:           "No Mounts",
			TestAgentMount:     &MountPoint{SourceVolumeID: TestMountPointVolume, ReadOnly: true},
			TestMountCount:     0,
			ExpectedMountCount: 0,
		},
		{
			TestName:           "Single Mount",
			TestAgentMount:     &MountPoint{SourceVolumeID: TestMountPointVolume, ReadOnly: true},
			TestMountCount:     1,
			ExpectedMountCount: 1,
		},
		{
			TestName:           "Duplicate SourceVolumeID Mounts Last Mount Wins",
			TestAgentMount:     &MountPoint{SourceVolumeID: TestMountPointVolume},
			TestMountCount:     2,
			ExpectedMountCount: 1,
		},
		{
			TestName:           "Duplicate SourceVolumeID applicationLogMount Last Mount Wins",
			TestAgentMount:     &MountPoint{SourceVolumeID: "applicationLogMount"},
			TestMountCount:     2,
			ExpectedMountCount: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			tmd := NewManagedDaemon(TestImageName, TestImageTag)
			mountPoints := []*MountPoint{}
			mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "agentCommunicationMount"})
			mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "applicationLogMount"})
			for i := 0; i < c.TestMountCount; i++ {
				mountPoints = append(mountPoints, c.TestAgentMount)
			}
			tmd.SetMountPoints(mountPoints)
			assert.Equal(t, c.ExpectedMountCount, len(tmd.GetFilteredMountPoints()), "Wrong value for Set Managed Daemon Mounts")
			// validate required mount points
			expectedAgentCommunicationMount := fmt.Sprintf(ExpectedAgentCommunicationMountFormat, TestImageName)
			expectedApplicationLogMount := fmt.Sprintf(ExpectedApplicationLogMountFormat, TestImageName)
			assert.Equal(t, expectedAgentCommunicationMount, tmd.GetAgentCommunicationMount().SourceVolumeHostPath)
			assert.Equal(t, expectedApplicationLogMount, tmd.GetApplicationLogMount().SourceVolumeHostPath)
		})
	}
}

func TestAddMountPoint(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "agentCommunicationMount"})
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "applicationLogMount"})
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetMountPoints(mountPoints)
	// test add valid mount point
	errResult := tmd.AddMountPoint(testMountPoint2)
	assert.Equal(t, 2, len(tmd.GetFilteredMountPoints()))
	assert.Equal(t, "TestMountPointVolume2", tmd.GetFilteredMountPoints()[1].SourceVolume)
	assert.Nil(t, errResult)

	// test add existing mount point -- should fail
	errResult = tmd.AddMountPoint(testMountPoint2)
	expectedErrorMsg := "MountPoint already exists at index 1"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestGetMountPointsFilteredUnfiltered(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "agentCommunicationMount"})
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "applicationLogMount"})
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetMountPoints(mountPoints)
	// test add valid mount point
	errResult := tmd.AddMountPoint(testMountPoint2)
	assert.Equal(t, 2, len(tmd.GetFilteredMountPoints()))
	assert.Equal(t, 4, len(tmd.GetMountPoints()))
	assert.Nil(t, errResult)

	// test add existing mount point -- should fail
	errResult = tmd.AddMountPoint(testMountPoint2)
	expectedErrorMsg := "MountPoint already exists at index 1"
	assert.EqualErrorf(t, errResult, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, errResult)
}

func TestUpdateMountPoint(t *testing.T) {
	testMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1"}
	mountPoints := []*MountPoint{}
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "agentCommunicationMount"})
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "applicationLogMount"})
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetMountPoints(mountPoints)
	assert.Equal(t, 1, len(tmd.GetFilteredMountPoints()))
	assert.False(t, tmd.GetFilteredMountPoints()[0].ReadOnly)

	// test update existing mount point
	updatedMountPoint1 := &MountPoint{SourceVolume: "TestMountPointVolume1", ReadOnly: true}
	errResult := tmd.UpdateMountPointBySourceVolume(updatedMountPoint1)
	assert.Equal(t, 1, len(tmd.GetFilteredMountPoints()))
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
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "agentCommunicationMount"})
	mountPoints = append(mountPoints, &MountPoint{SourceVolumeID: "applicationLogMount"})
	mountPoints = append(mountPoints, testMountPoint1)
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetMountPoints(mountPoints)
	assert.Equal(t, 1, len(tmd.GetFilteredMountPoints()))
	assert.False(t, tmd.GetMountPoints()[0].ReadOnly)
	// test delete non-existing mount point
	testMountPoint2 := &MountPoint{SourceVolume: "TestMountPointVolume2"}
	errResult := tmd.DeleteMountPoint(testMountPoint2)
	assert.Equal(t, 1, len(tmd.GetFilteredMountPoints()))
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
			tmd := NewManagedDaemon(TestImageName, TestImageTag)
			tmd.SetEnvironment(c.TestEnvironmentMap)
			assert.Equal(t, len(c.TestEnvironmentMap), len(tmd.GetEnvironment()), "Wrong value for Set Environment")
		})
	}
}

func TestAddEnvVar(t *testing.T) {
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
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
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
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
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
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
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetHealthCheck(testHealthCheck, testHealthInterval, testHealthTimeout, testHealthRetries)
	dockerCheck := tmd.GetDockerHealthConfig()
	assert.Equal(t, expectedDockerCheck, dockerCheck)
}

func TestIsValidManagedDaemon(t *testing.T) {
	testAgentCommunicationMount := &MountPoint{SourceVolumeID: "agentCommunicationMount"}
	testApplicationLogMount := &MountPoint{SourceVolumeID: "applicationLogMount"}
	cases := []struct {
		TestName       string
		TestDaemon     *ManagedDaemon
		ExpectedResult bool
	}{
		{
			TestName: "All Valid",
			TestDaemon: &ManagedDaemon{agentCommunicationMount: testAgentCommunicationMount,
				applicationLogMount: testApplicationLogMount},
			ExpectedResult: true,
		},
		{
			TestName:       "Missing Required Agent communication Mount",
			TestDaemon:     &ManagedDaemon{applicationLogMount: testApplicationLogMount},
			ExpectedResult: false,
		},
		{
			TestName:       "Missing Required Log Mount",
			TestDaemon:     &ManagedDaemon{agentCommunicationMount: testAgentCommunicationMount},
			ExpectedResult: false,
		},
	}
	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			assert.Equal(t, c.TestDaemon.IsValidManagedDaemon(), c.ExpectedResult, "Wrong result for valid daemon check")
		})
	}
}

func TestSetContainerId(t *testing.T) {
	testContainerId := "testContainerId"
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetContainerId(testContainerId)
	assert.Equal(t, testContainerId, tmd.GetContainerId(), "Wrong value for set ContainerId")
}

func TestSetContainerCGroup(t *testing.T) {
	testContainerCGroup := "testContainerCGroup"
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetContainerCGroup(testContainerCGroup)
	assert.Equal(t, testContainerCGroup, tmd.GetContainerCGroup(), "Wrong value for set ContainerCGroup")
}

func TestSetNetworkNameSpace(t *testing.T) {
	testNetworkNameSpace := "testNetworkNameSpace"
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetNetworkNameSpace(testNetworkNameSpace)
	assert.Equal(t, testNetworkNameSpace, tmd.GetNetworkNameSpace(), "Wrong value for set NetworkNameSpace")
}

func TestSetCommand(t *testing.T) {
	testCommand := []string{"testCommand1", "testCommand2"}
	tmd := NewManagedDaemon(TestImageName, TestImageTag)
	tmd.SetCommand(testCommand)
	assert.Equal(t, testCommand, tmd.GetCommand(), "Wrong value for set Command")
}
