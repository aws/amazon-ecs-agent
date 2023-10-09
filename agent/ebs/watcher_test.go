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

	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	mock_ebs_discovery "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
	md "github.com/aws/amazon-ecs-agent/ecs-agent/manageddaemon"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	resourceAttachmentARN = "arn:aws:ecs:us-west-2:123456789012:attachment/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	containerInstanceARN  = "arn:aws:ecs:us-west-2:123456789012:container-instance/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	taskARN               = "task1"
	taskClusterARN        = "arn:aws:ecs:us-west-2:123456789012:cluster/customer-task-cluster"
)

// newTestEBSWatcher creates a new EBSWatcher object for testing
func newTestEBSWatcher(ctx context.Context, agentState dockerstate.TaskEngineState,
	discoveryClient apiebs.EBSDiscovery, taskEngine engine.TaskEngine) *EBSWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      agentState,
		discoveryClient: discoveryClient,
		taskEngine:      taskEngine,
	}
}

// TestHandleEBSAttachmentHappyCase tests handling a new resource attachment of type Elastic Block Stores
// The expected behavior is for the resource attachment to be added to the agent state and be able to be marked as attached.
func TestHandleEBSAttachmentHappyCase(t *testing.T) {
	t.Skip()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	testAttachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)
	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(taskresourcevolume.TestDeviceName, nil).
		MinTimes(1)

	err := watcher.HandleEBSResourceAttachment(ebsAttachment)
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
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	assert.True(t, ok)
	assert.True(t, ebsAttachment.IsAttached())
}

// TestHandleExpiredEBSAttachment tests acknowledging an expired resource attachment of type Elastic Block Stores
// The resource attachment object should not be saved to the agent state since the expiration date is in the past.
func TestHandleExpiredEBSAttachment(t *testing.T) {
	t.Skip()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	testAttachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)

	err := watcher.HandleEBSResourceAttachment(ebsAttachment)
	assert.Error(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	assert.False(t, ok)
}

// TestHandleDuplicateEBSAttachment tests handling duplicate resource attachment of type Elastic Block Store
// The expected behavior is for only of the resource attachment object to be added to the agent state.
func TestHandleDuplicateEBSAttachment(t *testing.T) {
	t.Skip()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)

	testAttachmentProperties1 := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       apiebs.EBSTaskAttach,
	}

	testAttachmentProperties2 := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       apiebs.EBSTaskAttach,
	}

	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)
	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return(taskresourcevolume.TestDeviceName, nil).
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
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
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
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	testAttachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       "InvalidResourceType",
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)

	watcher.HandleResourceAttachment(ebsAttachment)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	_, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
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
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	testAttachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)

	watcher.HandleResourceAttachment(ebsAttachment)
	time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis * 2)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	assert.False(t, ok)
}

// TestHandleMismatchEBSAttachment tests handling an EBS attachment but found a different volume attached
// onto the host during the scanning process.
func TestHandleMismatchEBSAttachment(t *testing.T) {
	t.Skip()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil).AnyTimes()
	mockTaskEngine.EXPECT().GetDaemonManagers().Return(nil).AnyTimes()

	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine)

	testAttachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
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
		AttachmentType:       apiebs.EBSTaskAttach,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
		Do(func(deviceName, volumeID string) {
			wg.Done()
		}).
		Return("", fmt.Errorf("%w; expected EBS volume %s but found %s", apiebs.ErrInvalidVolumeID, taskresourcevolume.TestVolumeId, "vol-321")).
		MinTimes(1)

	err := watcher.HandleEBSResourceAttachment(ebsAttachment)
	assert.NoError(t, err)

	pendingEBS := watcher.agentState.GetAllPendingEBSAttachmentsWithKey()
	foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, watcher.discoveryClient)
	wg.Wait()
	assert.Empty(t, foundVolumes)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	require.True(t, ok)
	assert.ErrorIs(t, ebsAttachment.GetError(), apiebs.ErrInvalidVolumeID)
}

// TODO add StageAll test
