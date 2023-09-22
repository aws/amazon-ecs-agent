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
	"fmt"
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

// newTestEBSWatcher creates a new EBSWatcher object for testing
func newTestEBSWatcher(ctx context.Context, agentState dockerstate.TaskEngineState,
	ebsChangeEvent chan<- statechange.Event, discoveryClient apiebs.EBSDiscovery) *EBSWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      agentState,
		ebsChangeEvent:  ebsChangeEvent,
		discoveryClient: discoveryClient,
	}
}

// TestHandleEBSAttachmentHappyCase tests handling a new resource attachment of type Elastic Block Stores
// The expected behavior is for the resource attachment to be added to the agent state and be able to be marked as attached.
func TestHandleEBSAttachmentHappyCase(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

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
	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)
	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(nil).
		MinTimes(1)

	err := watcher.HandleResourceAttachment(ebsAttachment)
	assert.NoError(t, err)

	// Instead of starting the EBS watcher, we'll be mocking a tick of the EBS watcher's scan ticker.
	// Otherwise, the watcher will continue to run forever and the test will panic.
	wg.Add(1)
	go func() {
		defer wg.Done()
		pendingEBS := watcher.agentState.GetAllPendingEBSAttachmentsWithKey()
		if len(pendingEBS) > 0 {
			foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, watcher.discoveryClient)
			watcher.NotifyAttached(foundVolumes)
		}
	}()

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
	assert.True(t, ebsAttachment.IsAttached())
}

// TestHandleExpiredEBSAttachment tests acknowledging an expired resource attachment of type Elastic Block Stores
// The resource attachment object should not be saved to the agent state since the expiration date is in the past.
func TestHandleExpiredEBSAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	expiresAt := time.Now().Add(-1 * time.Millisecond)
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
	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)

	err := watcher.HandleResourceAttachment(ebsAttachment)
	assert.Error(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.False(t, ok)
}

// TestHandleDuplicateEBSAttachment tests handling duplicate resource attachment of type Elastic Block Store
// The expected behavior is for only of the resource attachment object to be added to the agent state.
func TestHandleDuplicateEBSAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)

	testAttachmentProperties1 := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	ebsAttachment1 := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               status.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties1,
	}

	testAttachmentProperties2 := map[string]string{
		apiebs.ResourceTypeName: apiebs.ElasticBlockStorage,
		apiebs.DeviceName:       deviceName,
		apiebs.VolumeIdName:     volumeID,
	}

	ebsAttachment2 := &apiebs.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               status.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties2,
	}

	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)
	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(nil).
		MinTimes(1)

	watcher.HandleResourceAttachment(ebsAttachment1)
	watcher.HandleResourceAttachment(ebsAttachment2)

	// Instead of starting the EBS watcher, we'll be mocking a tick of the EBS watcher's scan ticker.
	// Otherwise, the watcher will continue to run forever and the test will panic.
	wg.Add(1)
	go func() {
		defer wg.Done()
		pendingEBS := watcher.agentState.GetAllPendingEBSAttachmentsWithKey()
		if len(pendingEBS) > 0 {
			foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, watcher.discoveryClient)
			watcher.NotifyAttached(foundVolumes)
		}
	}()

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
	assert.True(t, ebsAttachment.IsAttached())
}

// TestHandleInvalidTypeEBSAttachment tests handling a resource attachment that is not of type Elastic Block Stores
// The expected behavior for the EBS watcher is to not add the resource attachment object to the agent state.
func TestHandleInvalidTypeEBSAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	testAttachmentProperties := map[string]string{
		apiebs.ResourceTypeName: "InvalidResourceType",
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
	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)

	watcher.HandleResourceAttachment(ebsAttachment)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.False(t, ok)
}

// TestHandleEBSAckTimeout tests acknowledging the timeout of a EBS-type resource attachment object saved within the agent state.
// The expected behavior is after the timeout duration, the resource attachment object will be removed from agent state.
// Skip flaky test until debugged
func TestHandleEBSAckTimeout(t *testing.T) {
	t.Skip("Skipping timeout test: flaky")
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

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
	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)

	watcher.HandleResourceAttachment(ebsAttachment)
	time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis * 2)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.False(t, ok)
}

// TestHandleMismatchEBSAttachment tests handling an EBS attachment but found a different volume attached
// onto the host during the scanning process.
func TestHandleMismatchEBSAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	watcher := newTestEBSWatcher(ctx, taskEngineState, eventChannel, mockDiscoveryClient)

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

	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(deviceName, volumeID).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(fmt.Errorf("%w; expected EBS volume %s but found %s", apiebs.ErrInvalidVolumeID, volumeID, "vol-321")).
		MinTimes(1)

	err := watcher.HandleResourceAttachment(ebsAttachment)
	assert.NoError(t, err)

	pendingEBS := watcher.agentState.GetAllPendingEBSAttachmentsWithKey()
	foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, watcher.discoveryClient)

	assert.Empty(t, foundVolumes)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
	assert.ErrorIs(t, ebsAttachment.GetError(), apiebs.ErrInvalidVolumeID)
}
