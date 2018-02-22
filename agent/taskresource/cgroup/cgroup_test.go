// +build linux
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

package taskresource

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/factory/mock"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/containerd/cgroups"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
)

const (
	validTaskArn    = "arn:aws:ecs:region:account-id:task/task-id"
	invalidTaskArn  = "invalid:task::arn"
	cgroupMountPath = "/sys/fs/cgroup"
	taskName        = "sleep5TaskCgroup"
)

func TestCreateHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil),
		mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil),
	)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	cgroupResource.ioutil = mockIO
	assert.NoError(t, cgroupResource.Create(task))
}

func TestCreateCgroupPathExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(true),
	)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	cgroupResource.ioutil = mockIO
	assert.NoError(t, cgroupResource.Create(task))
}

func TestCreateInvalidResourceSpec(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	task := testdata.LoadTask(taskName)
	task.CPU = float64(100)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
	)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	cgroupResource.ioutil = mockIO
	assert.Error(t, cgroupResource.Create(task))
}

func TestCreateCgroupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockCgroup := mock_cgroups.NewMockCgroup(ctrl)

	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(mockCgroup, errors.New("cgroup create error")),
	)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	cgroupResource.ioutil = mockIO
	assert.Error(t, cgroupResource.Create(task))
}

func TestCleanupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(cgroupRoot).Return(nil)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	assert.NoError(t, cgroupResource.Cleanup())
}

func TestCleanupRemoveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(gomock.Any()).Return(errors.New("cgroup remove error"))

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	assert.Error(t, cgroupResource.Cleanup())
}

func TestCleanupCgroupDeletedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	task := testdata.LoadTask(taskName)
	taskID, err := task.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)

	mockControl.EXPECT().Remove(gomock.Any()).Return(cgroups.ErrCgroupDeleted)

	cgroupResource := NewCgroupResource(mockControl, cgroupRoot, cgroupMountPath)
	assert.NoError(t, cgroupResource.Cleanup())
}

func TestMarshal(t *testing.T) {
	cgroupStr := "{\"CgroupRoot\":\"/ecs/taskid\",\"CgroupMountPath\":\"/sys/fs/cgroup\"," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}"

	cgroupRoot := "/ecs/taskid"
	cgroupMountPath := "/sys/fs/cgroup"

	cgroup := NewCgroupResource(cgroup.New(), cgroupRoot, cgroupMountPath)
	cgroup.SetDesiredStatus(CgroupCreated)
	cgroup.SetKnownStatus(CgroupStatusNone)

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

	assert.Equal(t, cgroupRoot, unmarshalledCgroup.CgroupRoot)
	assert.Equal(t, cgroupMountPath, unmarshalledCgroup.CgroupMountPath)
	assert.Equal(t, time.Time{}, unmarshalledCgroup.GetCreatedAt())
	assert.Equal(t, CgroupCreated, unmarshalledCgroup.GetDesiredStatus())
	assert.Equal(t, CgroupStatusNone, unmarshalledCgroup.GetKnownStatus())
}
