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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const (
	taskARN       = "task1"
	volumeName    = "volumeName1"
	targetDir     = "/data/efs/targetDir"
	rootDir       = "source"
	mountOptions  = "mountOptions"
	readOnly      = true
	testDNS       = "endpoint1"
	testMountIP   = "127.0.0.1"
	containerName = "testContainerName"
	dockerName    = "testDockerName"
	dataDirOnHost = "/var/lib/ecs/data"
)

func TestEFSConfigSource(t *testing.T) {
	config := EFSConfig{
		TargetDir:     targetDir,
		DataDirOnHost: dataDirOnHost,
	}
	assert.Equal(t, "/var/lib/ecs/data/efs/targetDir", config.Source())
}

func resetMount() {
	mountSyscall = unix.Mount
	unmountSyscall = unix.Unmount
}

func TestCreateSuccess(t *testing.T) {
	defer resetHelper()
	defer resetMount()
	lookUpHostHelper = lookUpHostHelperFunction([]string{testMountIP}, nil)
	d := fmt.Sprintf("%s:/%s", testMountIP, rootDir)
	mountSyscall = func(device string, target string, fstype string, flags uintptr, opts string) error {
		assert.Equal(t, d, device)
		assert.Equal(t, targetDir, target)
		assert.Equal(t, "nfs", fstype)
		assert.Equal(t, uintptr(0), flags)
		assert.Equal(t, "rsize=1048576,wsize=1048576,timeo=10,hard,retrans=2,noresvport,vers=4,addr=127.0.0.1", opts)
		return nil
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	dnsEndpoints := []string{testDNS}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		awsvpcMode: false,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(targetDir, os.ModePerm),
	)

	assert.NoError(t, testRes.Create())
	assert.NotNil(t, testRes.getNFSMount())
}

func TestCreateAlreadyMounted(t *testing.T) {
	defer resetHelper()
	defer resetMount()
	lookUpHostHelper = lookUpHostHelperFunction([]string{testMountIP}, nil)
	mountSyscall = func(device string, target string, fstype string, flags uintptr, opts string) error {
		return fmt.Errorf("%s is busy or already mounted", targetDir)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	dnsEndpoints := []string{testDNS}

	testRes := &EFSResource{
		awsvpcMode: false,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(targetDir, os.ModePerm),
	)

	assert.NoError(t, testRes.Create())
	assert.NotNil(t, testRes.getNFSMount())
}

func TestCreateFailureSetTerminalReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOS := mock_oswrapper.NewMockOS(ctrl)

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(targetDir, os.ModePerm).Return(errors.New("already exists")),
	)

	assert.Error(t, testRes.Create())
	assert.Equal(t, "unable to create mount target directory for efs: already exists", testRes.GetTerminalReason())
}

func TestInitMountNotAWSVPCMode(t *testing.T) {
	defer resetHelper()
	lookUpHostHelper = lookUpHostHelperFunction([]string{testMountIP}, nil)

	dnsEndpoints := []string{testDNS}
	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		awsvpcMode: false,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	mHandler, _ := testRes.initMount()
	assert.Equal(t, "", mHandler.NamespacePath)
	assert.Equal(t, testMountIP, mHandler.IPAddress)
	assert.Equal(t, targetDir, mHandler.TargetDirectory)
	assert.Equal(t, rootDir, mHandler.SourceDirectory)
}

func TestInitMountAWSVPCMode(t *testing.T) {
	defer resetHelper()
	lookUpHostHelper = lookUpHostHelperFunction([]string{testMountIP}, nil)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dnsEndpoints := []string{testDNS}

	testRes := &EFSResource{
		taskARN:            taskARN,
		volumeName:         volumeName,
		ctx:                ctx,
		client:             mockClient,
		pauseContainerName: containerName,
		awsvpcMode:         true,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockClient.EXPECT().InspectContainer(ctx, containerName, dockerclient.InspectContainerTimeout).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{Pid: 1},
			},
		}, nil),
	)

	mHandler, _ := testRes.initMount()
	assert.Equal(t, "/host/proc/1/ns/net", mHandler.NamespacePath)
	assert.Equal(t, testMountIP, mHandler.IPAddress)
	assert.Equal(t, targetDir, mHandler.TargetDirectory)
	assert.Equal(t, rootDir, mHandler.SourceDirectory)
}

func TestCreateDir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	dnsEndpoints := []string{"endpoint1"}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(targetDir, os.ModePerm),
	)

	assert.NoError(t, testRes.createDir())
}

func TestCreateDirError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	dnsEndpoints := []string{"endpoint1"}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(targetDir, os.ModePerm).Return(errors.New("already exists")),
	)

	err := testRes.createDir()
	assert.NotNil(t, err)
	assert.Equal(t, "unable to create mount target directory for efs: already exists", err.Error())
}

func TestDNSResolve(t *testing.T) {
	defer resetHelper()
	dnsEndpoints := []string{testDNS}
	ips := []string{testMountIP}
	lookUpHostHelper = func(dns string) ([]string, error) {
		assert.Equal(t, testDNS, dns)
		return ips, nil
	}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	ip, err := testRes.dnsResolve()
	assert.Equal(t, testMountIP, ip)
	assert.Nil(t, err)
}

func TestDNSResolveError(t *testing.T) {
	defer resetHelper()
	dnsEndpoints := []string{testDNS}
	lookUpHostHelper = func(dns string) ([]string, error) {
		assert.Equal(t, testDNS, dns)
		return nil, errors.New("error")
	}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}

	_, err := testRes.dnsResolve()
	assert.Equal(t, "unable to resolve dns for efs: error", err.Error())
}

func TestSetOS(t *testing.T) {
	testRes := &EFSResource{
		taskARN: taskARN,
	}

	assert.Nil(t, testRes.os)
	testRes.setOS()
	assert.NotNil(t, testRes.os)
}

func TestCleanupWithMount(t *testing.T) {
	defer resetMount()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	unmountSyscall = func(target string, flags int) error {
		assert.Equal(t, targetDir, target)
		assert.Equal(t, flags, 0)
		return nil
	}

	mount := &NFSMount{
		TargetDirectory: targetDir,
		IPAddress:       testMountIP,
		NamespacePath:   "",
		SourceDirectory: rootDir,
	}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		awsvpcMode: false,
		nfsMount:   mount,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir: targetDir,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().RemoveAll(targetDir),
	)
	assert.NoError(t, testRes.Cleanup())
}

func TestCleanupWithoutMount(t *testing.T) {
	defer resetHelper()
	defer resetMount()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOS := mock_oswrapper.NewMockOS(ctrl)

	lookUpHostHelper = lookUpHostHelperFunction([]string{testMountIP}, nil)
	unmountSyscall = func(target string, flags int) error {
		assert.Equal(t, targetDir, target)
		assert.Equal(t, flags, 0)
		return nil
	}

	dnsEndpoints := []string{testDNS}
	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		awsvpcMode: false,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir:    targetDir,
			RootDir:      rootDir,
			DNSEndpoints: dnsEndpoints,
			MountOptions: mountOptions,
			ReadOnly:     readOnly,
		},
	}
	gomock.InOrder(
		mockOS.EXPECT().RemoveAll(targetDir),
	)
	assert.NoError(t, testRes.Cleanup())
}

func TestCleanupNotMounted(t *testing.T) {
	defer resetMount()
	unmountSyscall = func(target string, flags int) error {
		return fmt.Errorf("%s: not mounted", targetDir)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOS := mock_oswrapper.NewMockOS(ctrl)

	mount := &NFSMount{
		TargetDirectory: targetDir,
		IPAddress:       testMountIP,
		NamespacePath:   "",
		SourceDirectory: rootDir,
	}

	testRes := &EFSResource{
		taskARN:    taskARN,
		volumeName: volumeName,
		awsvpcMode: false,
		nfsMount:   mount,
		os:         mockOS,
		Config: EFSConfig{
			TargetDir: targetDir,
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().RemoveAll(targetDir),
	)
	assert.NoError(t, testRes.Cleanup())
}

func TestBuildContainerDependency(t *testing.T) {
	testRes := &EFSResource{
		taskARN:                   taskARN,
		transitionDependenciesMap: make(map[resourcestatus.ResourceStatus]taskresource.TransitionDependencySet),
	}

	assert.NoError(t, testRes.BuildContainerDependency(containerName, apicontainerstatus.ContainerCreated, resourcestatus.ResourceStatus(EFSCreated)))

	contDep := testRes.GetContainerDependencies(resourcestatus.ResourceStatus(EFSCreated))
	assert.NotNil(t, contDep)

	assert.Equal(t, containerName, contDep[0].ContainerName)
	assert.Equal(t, apicontainerstatus.ContainerCreated, contDep[0].SatisfiedStatus)

}

func TestMarshalUnmarshalJSON(t *testing.T) {
	efsConfig := EFSConfig{
		TargetDir:     targetDir,
		DataDirOnHost: "/data",
	}
	conDep := []apicontainer.ContainerDependency{
		{
			ContainerName:   containerName,
			SatisfiedStatus: apicontainerstatus.ContainerCreated,
		},
	}
	transSet := taskresource.TransitionDependencySet{
		ContainerDependencies: conDep,
	}
	transMap := map[resourcestatus.ResourceStatus]taskresource.TransitionDependencySet{
		resourcestatus.ResourceStatus(EFSCreated): transSet,
	}
	efsResIn := &EFSResource{
		taskARN:                   taskARN,
		volumeName:                volumeName,
		pauseContainerName:        containerName,
		awsvpcMode:                false,
		Config:                    efsConfig,
		transitionDependenciesMap: transMap,
		createdAt:                 time.Now(),
		knownStatusUnsafe:         resourcestatus.ResourceCreated,
		desiredStatusUnsafe:       resourcestatus.ResourceCreated,
	}

	bytes, err := json.Marshal(efsResIn)
	require.NoError(t, err)

	efsResOut := &EFSResource{}
	err = json.Unmarshal(bytes, efsResOut)
	require.NoError(t, err)

	assert.Equal(t, efsResIn.taskARN, efsResOut.taskARN)
	assert.Equal(t, efsResIn.volumeName, efsResOut.volumeName)
	assert.Equal(t, efsResIn.pauseContainerName, efsResOut.pauseContainerName)
	assert.Equal(t, efsResIn.awsvpcMode, efsResOut.awsvpcMode)
	assert.WithinDuration(t, efsResIn.createdAt, efsResOut.createdAt, time.Microsecond)
	assert.Equal(t, efsResIn.desiredStatusUnsafe, efsResOut.desiredStatusUnsafe)
	assert.Equal(t, efsResIn.knownStatusUnsafe, efsResOut.knownStatusUnsafe)
	assert.Equal(t, targetDir, efsConfig.Source())

	outConDep := efsResOut.GetContainerDependencies(resourcestatus.ResourceStatus(EFSCreated))
	assert.Equal(t, containerName, outConDep[0].ContainerName)
	assert.Equal(t, apicontainerstatus.ContainerCreated, outConDep[0].SatisfiedStatus)
}

func lookUpHostHelperFunction(ips []string, err error) func(string) ([]string, error) {
	return func(string) ([]string, error) {
		return ips, err
	}

}

func resetHelper() {
	lookUpHostHelper = net.LookupHost
}
