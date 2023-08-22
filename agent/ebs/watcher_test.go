//go:build unit
// +build unit

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

package ebs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	mock_ebs_discovery "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	resourceAttachmentARN = "arn:aws:ecs:us-west-2:123456789012:attachment/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	containerInstanceARN  = "arn:aws:ecs:us-west-2:123456789012:container-instance/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	taskARN               = "task1"
	taskClusterARN        = "arn:aws:ecs:us-west-2:123456789012:cluster/customer-task-cluster"
	deviceName            = "/dev/xvdba"
	volumeID              = "vol-1234"
)

type TestGroup struct {
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewTestGroup creates a TestGroup with the given cancellation function.
func NewTestGroup(cancel context.CancelFunc) TestGroup {
	return TestGroup{cancel: cancel}
}

// Start the watcher in a new goroutine. The watcher must be initialized with
// the group's context.
func (tg *TestGroup) Start(watcher *EBSWatcher) {
	tg.wg.Add(1)
	go func() {
		defer tg.wg.Done()
		watcher.Start()
	}()
}

// Cancel the watcher started by Start() by calling the associated context and wait its completion.
func (tg *TestGroup) Cancel() {
	// Stop the watcher.
	tg.cancel()

	// Make sure all watcher have been stopped.
	tg.wg.Wait()
}

func setupWatcher(ctx context.Context, cancel context.CancelFunc, agentState dockerstate.TaskEngineState,
	ebsChangeEvent chan<- statechange.Event, discoveryClient apiebs.EBSDiscovery) *EBSWatcher {
	return &EBSWatcher{
		ctx:             ctx,
		cancel:          cancel,
		agentState:      agentState,
		ebsChangeEvent:  ebsChangeEvent,
		discoveryClient: discoveryClient,
		mailbox:         make(chan func(), 1),
	}
}

func TestHandleEBSAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	testGroup := NewTestGroup(cancel)
	defer testGroup.Cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).Return(nil).MinTimes(1).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(nil).
		MinTimes(1)

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	ebsAttachment := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               status.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
	}

	watcher := setupWatcher(ctx, cancel, taskEngineState, eventChannel, mockDiscoveryClient)
	testGroup.Start(watcher)

	watcher.HandleResourceAttachment(ebsAttachment)

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
}

func TestHandleExpiredEBSAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	testGroup := NewTestGroup(cancel)
	defer testGroup.Cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).Return(nil).MinTimes(1).
		Return(errors.New("volume not found")).
		MaxTimes(1) // Expires after 100 ms, and we scan for volumes every 500 ms

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	expiresAt := time.Now().Add(time.Millisecond * 100)
	ebsAttachment := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
	}

	watcher := setupWatcher(ctx, cancel, taskEngineState, eventChannel, mockDiscoveryClient)
	testGroup.Start(watcher)

	watcher.HandleResourceAttachment(ebsAttachment)

	time.Sleep(600 * time.Millisecond)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.False(t, ok)
}

func TestHandleDuplicateEBSAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	testGroup := NewTestGroup(cancel)
	defer testGroup.Cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).Return(nil).MinTimes(1).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(nil).
		MinTimes(1)

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	ebsAttachment := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
	}

	watcher := setupWatcher(ctx, cancel, taskEngineState, eventChannel, mockDiscoveryClient)
	testGroup.Start(watcher)

	watcher.HandleResourceAttachment(ebsAttachment)
	watcher.HandleResourceAttachment(ebsAttachment)

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
}

func TestHandleInvalidEBSAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	testGroup := NewTestGroup(cancel)
	defer testGroup.Cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: "SomeOtherBlockStorage",
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	ebsAttachment := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               status.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
	}

	watcher := setupWatcher(ctx, cancel, taskEngineState, eventChannel, mockDiscoveryClient)
	testGroup.Start(watcher)

	watcher.HandleResourceAttachment(ebsAttachment)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.False(t, ok)
}
