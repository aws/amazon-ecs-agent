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
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager"
	mock_daemonmanager "github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager/mock"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	statechange "github.com/aws/amazon-ecs-agent/agent/statechange"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	mock_ebs_discovery "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	csi "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	mock_csiclient "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient/mocks"
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
	discoveryClient apiebs.EBSDiscovery, taskEngine engine.TaskEngine, csiClient csi.CSIClient) *EBSWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      agentState,
		discoveryClient: discoveryClient,
		taskEngine:      taskEngine,
		csiClient:       csiClient,
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

	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
	mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
		taskresourcevolume.TestVolumeId,
		gomock.Any(),
		filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
		taskresourcevolume.TestFileSystem,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any()).Return(nil).AnyTimes()

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
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
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)

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

	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
	mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
		taskresourcevolume.TestVolumeId,
		gomock.Any(),
		filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
		taskresourcevolume.TestFileSystem,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any()).Return(nil).AnyTimes()

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties2,
		AttachmentType:       apiebs.EBSTaskAttach,
	}

	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
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

func TestStageAll(t *testing.T) {
	tcs := []struct {
		name                     string
		setTaskStateExpectations func(*mock_dockerstate.MockTaskEngineState)
		setCSIClientExpectations func(*mock_csiclient.MockCSIClient)
		foundVolumes             map[string]string
		expectedErrorStubs       []string
		expectedAttachmentStatus attachment.AttachmentStatus
	}{
		{
			name: "StageAll succeeds with expected EBS CSI Driver fields",
			setTaskStateExpectations: func(ts *mock_dockerstate.MockTaskEngineState) {
				ebsResourceAttachment := getEBSResourceAttachment()
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(ebsResourceAttachment, true).Times(2)
			},
			setCSIClientExpectations: func(mcc *mock_csiclient.MockCSIClient) {
				mcc.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId, gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem, gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			foundVolumes:             map[string]string{taskresourcevolume.TestVolumeId: taskresourcevolume.TestDeviceName},
			expectedErrorStubs:       nil,
			expectedAttachmentStatus: attachment.AttachmentAttached,
		},
		{
			name: "StageAll returns expected error when CSI NodeStage call fails",
			setTaskStateExpectations: func(ts *mock_dockerstate.MockTaskEngineState) {
				ebsResourceAttachment := getEBSResourceAttachment()
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(ebsResourceAttachment, true).Times(2)
			},
			setCSIClientExpectations: func(mcc *mock_csiclient.MockCSIClient) {
				mcc.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId, gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem, gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("test")).Times(1)
			},
			foundVolumes:             map[string]string{taskresourcevolume.TestVolumeId: taskresourcevolume.TestDeviceName},
			expectedErrorStubs:       []string{"Failed to initialize EBS volume ID"},
			expectedAttachmentStatus: attachment.AttachmentNone,
		},
		{
			name: "StageAll returns nil when already attached",
			setTaskStateExpectations: func(ts *mock_dockerstate.MockTaskEngineState) {
				ebsResourceAttachment := getEBSResourceAttachment()
				ebsResourceAttachment.SetAttachedStatus()
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(ebsResourceAttachment, true).Times(2)
			},
			foundVolumes: map[string]string{taskresourcevolume.TestVolumeId: taskresourcevolume.TestDeviceName},
			// we won't call NodeStage here
			setCSIClientExpectations: func(mcc *mock_csiclient.MockCSIClient) {},
			expectedErrorStubs:       nil,
			expectedAttachmentStatus: attachment.AttachmentAttached,
		},
		{
			name: "StageAll fails with missing volume ID",
			setTaskStateExpectations: func(ts *mock_dockerstate.MockTaskEngineState) {
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(nil, false).Times(1)
				ebsResourceAttachment := getEBSResourceAttachment()
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(ebsResourceAttachment, true).Times(1)
			},
			foundVolumes:             map[string]string{taskresourcevolume.TestVolumeId: taskresourcevolume.TestDeviceName},
			setCSIClientExpectations: func(mcc *mock_csiclient.MockCSIClient) {},
			expectedErrorStubs:       []string{"Unable to find EBS volume with volume ID"},
			expectedAttachmentStatus: attachment.AttachmentNone,
		},
		{
			name: "StageAll multiple volumes returns a single error and continues if first volume stage fails",
			setTaskStateExpectations: func(ts *mock_dockerstate.MockTaskEngineState) {
				ts.EXPECT().GetEBSByVolumeId("testVolId").Return(nil, false).Times(1)
				ebsResourceAttachment := getEBSResourceAttachment()
				ts.EXPECT().GetEBSByVolumeId(taskresourcevolume.TestVolumeId).Return(ebsResourceAttachment, true).Times(2)
			},
			foundVolumes: map[string]string{"testVolId": "testDeviceName", taskresourcevolume.TestVolumeId: taskresourcevolume.TestDeviceName},
			setCSIClientExpectations: func(mcc *mock_csiclient.MockCSIClient) {
				mcc.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId, gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem, gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			expectedErrorStubs: []string{"Unable to find EBS volume with volume ID"},
			// note that this only checks the taskresourcevolume.TestVolumeId attachment status
			expectedAttachmentStatus: attachment.AttachmentAttached,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			ctx := context.Background()

			taskEngineState := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
			tc.setTaskStateExpectations(taskEngineState)
			mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
			mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)

			mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
			tc.setCSIClientExpectations(mockCsiClient)

			watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
			errors := watcher.StageAll(tc.foundVolumes)
			assert.Equal(t, len(tc.expectedErrorStubs), len(errors))
			if len(tc.expectedErrorStubs) != len(errors) {
				t.Errorf("mismatched error count")
			} else {
				for i, err := range tc.expectedErrorStubs {
					assert.ErrorContains(t, errors[i], err)
				}
			}
			ebsAttachment, _ := taskEngineState.GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
			assert.Equal(t, tc.expectedAttachmentStatus, ebsAttachment.GetAttachmentStatus())
		})
	}
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
	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       "InvalidResourceType",
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)

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
	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)

	watcher.HandleResourceAttachment(ebsAttachment)
	time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis * 2)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	assert.False(t, ok)
}

// TestHandleMismatchEBSAttachment tests handling an EBS attachment but found a different volume attached
// onto the host during the scanning process.
func TestHandleMismatchEBSAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)

	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
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

// TestHandleEBSAttachmentWithExistingCSIDriverTask tests handling an EBS attachment when there's already a known running CSI driver daemon
// task. There should be no calls to GetDaemonManagers nor CreateDaemonTask.
func TestHandleEBSAttachmentWithExistingCSIDriverTask(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
		Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
		KnownStatusUnsafe: status.TaskRunning,
	}).AnyTimes()
	mockTaskEngine.EXPECT().StateChangeEvents().Return(make(chan statechange.Event)).AnyTimes()

	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
	mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
		taskresourcevolume.TestVolumeId,
		gomock.Any(),
		filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
		taskresourcevolume.TestFileSystem,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any()).Return(nil).AnyTimes()

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
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
			watcher.StageAll(foundVolumes)
			watcher.NotifyAttached(foundVolumes)
		}
	}()

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	require.True(t, ok)
	assert.True(t, ebsAttachment.IsAttached())
}

// TestHandleEBSAttachmentWithStoppedCSIDriverTask tests handling an EBS attachment when there's an existing CSI driver daemon task
// saved to the task engine but is STOPPED. There should be a call to CreateDaemonTask which is suppose to create a new CSI driver task
// and will then be set and added to the task engine.
func TestHandleEBSAttachmentWithStoppedCSIDriverTask(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
		Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
		KnownStatusUnsafe: status.TaskStopped,
	}).AnyTimes()
	mockDaemonManager := mock_daemonmanager.NewMockDaemonManager(mockCtrl)
	mockDaemonManager.EXPECT().CreateDaemonTask().Return(&task.Task{
		Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
		KnownStatusUnsafe: status.TaskCreated,
	}, nil).AnyTimes()

	daemonManagers := map[string]daemonmanager.DaemonManager{
		md.EbsCsiDriver: mockDaemonManager,
	}

	mockTaskEngine.EXPECT().GetDaemonManagers().Return(daemonManagers).AnyTimes()
	mockTaskEngine.EXPECT().StateChangeEvents().Return(make(chan statechange.Event)).AnyTimes()
	mockTaskEngine.EXPECT().SetDaemonTask(md.EbsCsiDriver, gomock.Any()).Return().AnyTimes()
	mockTaskEngine.EXPECT().AddTask(gomock.Any()).Return().AnyTimes()

	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
	mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
		taskresourcevolume.TestVolumeId,
		gomock.Any(),
		filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
		taskresourcevolume.TestFileSystem,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any()).Return(nil).AnyTimes()

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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       apiebs.EBSTaskAttach,
	}
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
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
			watcher.StageAll(foundVolumes)
			watcher.NotifyAttached(foundVolumes)
		}
	}()

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
	require.True(t, ok)
	assert.True(t, ebsAttachment.IsAttached())
}

func TestDaemonRunning(t *testing.T) {
	tcs := []struct {
		name               string
		setTaskEngineMocks func(*gomock.Controller, *mock_engine.MockTaskEngine)
		expected           bool
	}{
		{
			name: "task is running",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				task := &task.Task{
					KnownStatusUnsafe: status.TaskRunning,
					Arn:               "arn:aws:ecs:us-west-2:1234:task/test/sometaskid",
				}
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(task)
			},
			expected: true,
		},
		{
			name: "task is created",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				task := &task.Task{
					KnownStatusUnsafe: status.TaskCreated,
					Arn:               "arn:aws:ecs:us-west-2:1234:task/test/sometaskid",
				}
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(task)
			},
			expected: false,
		},
		{
			name: "task is not created, daemon manager not found",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil)
				mte.EXPECT().GetDaemonManagers().Return(map[string]daemonmanager.DaemonManager{})
			},
			expected: false,
		},
		{
			name: "image not loaded yet",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil)
				daemonManager := mock_daemonmanager.NewMockDaemonManager(ctrl)
				daemonManager.EXPECT().IsLoaded(gomock.Any()).Return(false, nil)
				daemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				dms := map[string]daemonmanager.DaemonManager{md.EbsCsiDriver: daemonManager}
				mte.EXPECT().GetDaemonManagers().Return(dms)
			},
			expected: false,
		},
		{
			name: "daemon task create failed",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil)
				daemonManager := mock_daemonmanager.NewMockDaemonManager(ctrl)
				daemonManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
				daemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				dms := map[string]daemonmanager.DaemonManager{md.EbsCsiDriver: daemonManager}
				mte.EXPECT().GetDaemonManagers().Return(dms)
				daemonManager.EXPECT().CreateDaemonTask().Return(nil, errors.New("error"))
			},
			expected: false,
		},
		{
			name: "new daemon task created successfully",
			setTaskEngineMocks: func(ctrl *gomock.Controller, mte *mock_engine.MockTaskEngine) {
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil)
				daemonManager := mock_daemonmanager.NewMockDaemonManager(ctrl)
				daemonManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
				daemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				dms := map[string]daemonmanager.DaemonManager{md.EbsCsiDriver: daemonManager}
				mte.EXPECT().GetDaemonManagers().Return(dms)
				csiTask := &task.Task{Arn: "arn:aws:ecs:us-west-2:1234:task/test/sometaskid"}
				daemonManager.EXPECT().CreateDaemonTask().Return(csiTask, nil)
				mte.EXPECT().SetDaemonTask(md.EbsCsiDriver, csiTask).Return()
				mte.EXPECT().AddTask(csiTask).Return()
			},
			expected: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskEngine := mock_engine.NewMockTaskEngine(ctrl)
			tc.setTaskEngineMocks(ctrl, taskEngine)

			watcher := &EBSWatcher{taskEngine: taskEngine}
			assert.Equal(t, tc.expected, watcher.daemonRunning())
		})
	}
}

func TestTick(t *testing.T) {
	type testCase struct {
		name                           string
		pendingAttachments             map[string]*apiebs.ResourceAttachment
		setTaskEngineStateExpectations func(*mock_dockerstate.MockTaskEngineState)
		setTaskEngineExpectations      func(*mock_engine.MockTaskEngine)
		setDiscoveryClientExpectations func(*mock_ebs_discovery.MockEBSDiscovery)
		assertEBSAttachmentsState      func(t *testing.T, atts map[string]*apiebs.ResourceAttachment)
	}

	attachmentAlreadySentCase := func() testCase {
		attachment := &apiebs.ResourceAttachment{
			AttachmentInfo: attachment.AttachmentInfo{
				Status:           attachment.AttachmentAttached,
				AttachStatusSent: true,
			},
			AttachmentProperties: map[string]string{apiebs.DeviceNameKey: "device-name"},
		}
		return testCase{
			name:               "daemon running and volume already attached and sent",
			pendingAttachments: map[string]*apiebs.ResourceAttachment{"ebs-volume:id": attachment},
			setTaskEngineStateExpectations: func(mtes *mock_dockerstate.MockTaskEngineState) {
				mtes.EXPECT().GetEBSByVolumeId("id").Return(attachment, true).Times(3)
			},
			setTaskEngineExpectations: func(mte *mock_engine.MockTaskEngine) {
				task := &task.Task{KnownStatusUnsafe: status.TaskRunning}
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(task)
			},
			setDiscoveryClientExpectations: func(me *mock_ebs_discovery.MockEBSDiscovery) {
				me.EXPECT().ConfirmEBSVolumeIsAttached("device-name", "id").Return("actual-name", nil)
			},
			assertEBSAttachmentsState: func(t *testing.T, atts map[string]*apiebs.ResourceAttachment) {
				attachment, ok := atts["ebs-volume:id"]
				require.True(t, ok, "attachment not found")
				assert.NoError(t, attachment.GetError())
				assert.True(t, attachment.IsAttached())
				assert.True(t, attachment.IsSent())
			},
		}
	}

	tcs := []testCase{
		{
			name:               "no-op when there are no pending attachments",
			pendingAttachments: map[string]*apiebs.ResourceAttachment{},
		},
		{
			name:               "no-op when daemon has been initialized but pending to run",
			pendingAttachments: map[string]*apiebs.ResourceAttachment{"id": &apiebs.ResourceAttachment{}},
			setTaskEngineExpectations: func(mte *mock_engine.MockTaskEngine) {
				task := &task.Task{KnownStatusUnsafe: status.TaskCreated}
				mte.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(task)
			},
			assertEBSAttachmentsState: func(t *testing.T, atts map[string]*apiebs.ResourceAttachment) {
				attachment, ok := atts["id"]
				require.True(t, ok, "attachment not found")
				assert.NoError(t, attachment.GetError())
				assert.False(t, attachment.IsAttached())
				assert.False(t, attachment.IsSent())
			},
		},
		attachmentAlreadySentCase(),
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskEngineState := mock_dockerstate.NewMockTaskEngineState(ctrl)
			taskEngine := mock_engine.NewMockTaskEngine(ctrl)
			dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
			discoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(ctrl)

			attachments := tc.pendingAttachments
			taskEngineState.EXPECT().GetAllPendingEBSAttachmentsWithKey().Return(attachments)
			if tc.setTaskEngineStateExpectations != nil {
				tc.setTaskEngineStateExpectations(taskEngineState)
			}
			if tc.setTaskEngineExpectations != nil {
				tc.setTaskEngineExpectations(taskEngine)
			}
			if tc.setDiscoveryClientExpectations != nil {
				tc.setDiscoveryClientExpectations(discoveryClient)
			}

			watcher := NewWatcher(context.Background(), taskEngineState, taskEngine, dockerClient)
			watcher.discoveryClient = discoveryClient
			watcher.tick()

			if tc.assertEBSAttachmentsState != nil {
				tc.assertEBSAttachmentsState(t, attachments)
			}
		})
	}
}

func getEBSResourceAttachment() *apiebs.ResourceAttachment {
	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	testAttachmentProperties1 := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
	}

	return &apiebs.ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:              taskARN,
			TaskClusterARN:       taskClusterARN,
			ContainerInstanceARN: containerInstanceARN,
			ExpiresAt:            expiresAt,
			Status:               attachment.AttachmentNone,
			AttachmentARN:        resourceAttachmentARN,
		},
		AttachmentProperties: testAttachmentProperties1,
		AttachmentType:       apiebs.EBSTaskAttach,
	}
}
