// +build linux,unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package cgroup

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control/factory/mock"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/containerd/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
)

const (
	validTaskArn    = "arn:aws:ecs:region:account-id:task/task-id"
	invalidTaskArn  = "invalid:task::arn"
	cgroupMountPath = "/sys/fs/cgroup"
	taskName        = "sleep5TaskCgroup"
	taskID          = "taskID"
)

func TestCreateHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil),
		mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil),
	)
	cgroupResource := NewCgroupResource("taskArn", mockControl, mockIO, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.NoError(t, cgroupResource.Create())
}

func TestCreateCgroupPathExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(true),
	)

	cgroupResource := NewCgroupResource("taskArn", mockControl, mockIO, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.NoError(t, cgroupResource.Create())
}

func TestCreateCgroupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockCgroup := mock_cgroups.NewMockCgroup(ctrl)

	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(mockCgroup, errors.New("cgroup create error")),
	)

	cgroupResource := NewCgroupResource("taskArn", mockControl, mockIO, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.Error(t, cgroupResource.Create())
}

func TestCleanupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(cgroupRoot).Return(nil)

	cgroupResource := NewCgroupResource("taskArn", mockControl, nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.NoError(t, cgroupResource.Cleanup())
}

func TestCleanupRemoveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(gomock.Any()).Return(errors.New("cgroup remove error"))

	cgroupResource := NewCgroupResource("taskArn", mockControl, nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.Error(t, cgroupResource.Cleanup())
}

func TestCleanupCgroupDeletedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(gomock.Any()).Return(cgroups.ErrCgroupDeleted)

	cgroupResource := NewCgroupResource("taskArn", mockControl, nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	assert.NoError(t, cgroupResource.Cleanup())
}

func TestMarshal(t *testing.T) {
	cgroupStr := "{\"cgroupRoot\":\"/ecs/taskid\",\"cgroupMountPath\":\"/sys/fs/cgroup\"," +
		"\"createdAt\":\"0001-01-01T00:00:00Z\",\"desiredStatus\":\"CREATED\",\"knownStatus\":\"NONE\",\"resourceSpec\":{}}"

	cgroupRoot := "/ecs/taskid"
	cgroupMountPath := "/sys/fs/cgroup"

	cgroup := NewCgroupResource("", cgroup.New(), nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	cgroup.SetDesiredStatus(taskresource.ResourceStatus(CgroupCreated))
	cgroup.SetKnownStatus(taskresource.ResourceStatus(CgroupStatusNone))

	bytes, err := cgroup.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, cgroupStr, string(bytes[:]))
}

func TestUnmarshal(t *testing.T) {
	cgroupRoot := "/ecs/taskid"
	cgroupMountPath := "/sys/fs/cgroup"
	bytes := []byte("{\"CgroupRoot\":\"/ecs/taskid\",\"CgroupMountPath\":\"/sys/fs/cgroup\"," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}")
	unmarshalledCgroup := &CgroupResource{}
	err := unmarshalledCgroup.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	assert.Equal(t, cgroupRoot, unmarshalledCgroup.GetCgroupRoot())
	assert.Equal(t, cgroupMountPath, unmarshalledCgroup.GetCgroupMountPath())
	assert.Equal(t, time.Time{}, unmarshalledCgroup.GetCreatedAt())
	assert.Equal(t, taskresource.ResourceStatus(CgroupCreated), unmarshalledCgroup.GetDesiredStatus())
	assert.Equal(t, taskresource.ResourceStatus(CgroupStatusNone), unmarshalledCgroup.GetKnownStatus())
}
