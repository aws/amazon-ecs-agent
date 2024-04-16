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

func TestHandleEBSAttachment(t *testing.T) {
	tcs := []struct {
		name                          string
		shouldTick                    bool
		shouldHandle                  bool
		setDaemonRunningExpectations  func(*gomock.Controller, *mock_engine.MockTaskEngine, *mock_daemonmanager.MockDaemonManager, chan statechange.Event)
		setCSIClientExpectations      func(*gomock.Controller, *mock_csiclient.MockCSIClient)
		setConfirmEBSVolumeIsAttached func(*gomock.Controller, *mock_ebs_discovery.MockEBSDiscovery)
		expectedNumSavedAttachments   int
		attachments                   []*apiebs.ResourceAttachment
		numTicks                      int
	}{
		{
			name:         "handle with new CSI driver task",
			shouldTick:   true,
			numTicks:     2,
			shouldHandle: true,
			setDaemonRunningExpectations: func(ctrl *gomock.Controller, mockTaskEngine *mock_engine.MockTaskEngine, mockDaemonManager *mock_daemonmanager.MockDaemonManager, stateChangeChan chan statechange.Event) {
				daemonManagers := map[string]daemonmanager.DaemonManager{
					md.EbsCsiDriver: mockDaemonManager,
				}
				gomock.InOrder(
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil),
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
						Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
						KnownStatusUnsafe: status.TaskRunning,
					}),
				)
				mockTaskEngine.EXPECT().GetDaemonManagers().Return(daemonManagers)
				mockDaemonManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
				mockDaemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				mockDaemonManager.EXPECT().CreateDaemonTask().Return(&task.Task{
					Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
					KnownStatusUnsafe: status.TaskRunning,
				}, nil)
				mockTaskEngine.EXPECT().SetDaemonTask(md.EbsCsiDriver, gomock.Any()).Return()
				mockTaskEngine.EXPECT().AddTask(gomock.Any()).Return()
				mockTaskEngine.EXPECT().StateChangeEvents().Return(stateChangeChan)
			},
			setCSIClientExpectations: func(ctrl *gomock.Controller, mockCsiClient *mock_csiclient.MockCSIClient) {
				mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId,
					gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any()).Return(nil)
			},
			setConfirmEBSVolumeIsAttached: func(ctrl *gomock.Controller, mockDiscoveryClient *mock_ebs_discovery.MockEBSDiscovery) {
				var wg sync.WaitGroup
				wg.Add(1)
				mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
					Do(func(deviceName, volumeID string) {
						wg.Done()
					}).
					Return(taskresourcevolume.TestDeviceName, nil)
			},
			expectedNumSavedAttachments: 1,
			attachments: []*apiebs.ResourceAttachment{
				getEBSResourceAttachment(),
			},
		},
		{
			name:                        "handle expired EBS attachment",
			shouldTick:                  false,
			shouldHandle:                false,
			expectedNumSavedAttachments: 0,
			attachments: []*apiebs.ResourceAttachment{
				{
					AttachmentInfo: attachment.AttachmentInfo{
						TaskARN:              taskARN,
						TaskClusterARN:       taskClusterARN,
						ContainerInstanceARN: containerInstanceARN,
						ExpiresAt:            time.Now().Add(-1 * time.Millisecond),
						Status:               attachment.AttachmentNone,
						AttachmentARN:        resourceAttachmentARN,
					},
					AttachmentProperties: getEBSAttachmentProperties(),
					AttachmentType:       apiebs.EBSTaskAttach,
				},
			},
		},
		{
			name:         "handle duplicate EBS attachments",
			shouldTick:   true,
			numTicks:     2,
			shouldHandle: true,
			setDaemonRunningExpectations: func(ctrl *gomock.Controller, mockTaskEngine *mock_engine.MockTaskEngine, mockDaemonManager *mock_daemonmanager.MockDaemonManager, stateChangeChan chan statechange.Event) {
				daemonManagers := map[string]daemonmanager.DaemonManager{
					md.EbsCsiDriver: mockDaemonManager,
				}
				gomock.InOrder(
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(nil),
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
						Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
						KnownStatusUnsafe: status.TaskRunning,
					}),
				)
				mockTaskEngine.EXPECT().GetDaemonManagers().Return(daemonManagers)
				mockDaemonManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
				mockDaemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				mockDaemonManager.EXPECT().CreateDaemonTask().Return(&task.Task{
					Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
					KnownStatusUnsafe: status.TaskRunning,
				}, nil)
				mockTaskEngine.EXPECT().SetDaemonTask(md.EbsCsiDriver, gomock.Any()).Return()
				mockTaskEngine.EXPECT().AddTask(gomock.Any()).Return()
				mockTaskEngine.EXPECT().StateChangeEvents().Return(stateChangeChan)
			},
			setCSIClientExpectations: func(ctrl *gomock.Controller, mockCsiClient *mock_csiclient.MockCSIClient) {
				mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId,
					gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any()).Return(nil)
			},
			setConfirmEBSVolumeIsAttached: func(ctrl *gomock.Controller, mockDiscoveryClient *mock_ebs_discovery.MockEBSDiscovery) {
				var wg sync.WaitGroup
				wg.Add(1)
				mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
					Do(func(deviceName, volumeID string) {
						wg.Done()
					}).
					Return(taskresourcevolume.TestDeviceName, nil)
			},
			expectedNumSavedAttachments: 1,
			attachments: []*apiebs.ResourceAttachment{
				getEBSResourceAttachment(),
				getEBSResourceAttachment(),
			},
		},
		{
			name:                        "handle non-EBS type attachment",
			shouldTick:                  false,
			shouldHandle:                true,
			expectedNumSavedAttachments: 0,
			attachments: []*apiebs.ResourceAttachment{
				{
					AttachmentInfo: attachment.AttachmentInfo{
						TaskARN:              taskARN,
						TaskClusterARN:       taskClusterARN,
						ContainerInstanceARN: containerInstanceARN,
						ExpiresAt:            time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis),
						Status:               attachment.AttachmentNone,
						AttachmentARN:        resourceAttachmentARN,
					},
					AttachmentProperties: getEBSAttachmentProperties(),
					AttachmentType:       "InvalidResourceType",
				},
			},
		},
		{
			name:         "handle EBS attachment with existing CSI driver",
			shouldTick:   true,
			numTicks:     1,
			shouldHandle: true,
			setDaemonRunningExpectations: func(ctrl *gomock.Controller, mockTaskEngine *mock_engine.MockTaskEngine, mockDaemonManager *mock_daemonmanager.MockDaemonManager, stateChangeChan chan statechange.Event) {
				mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
					Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
					KnownStatusUnsafe: status.TaskRunning,
				})
				mockTaskEngine.EXPECT().StateChangeEvents().Return(stateChangeChan)
			},
			setCSIClientExpectations: func(ctrl *gomock.Controller, mockCsiClient *mock_csiclient.MockCSIClient) {
				mockCsiClient.EXPECT().NodeStageVolume(gomock.Any(),
					taskresourcevolume.TestVolumeId,
					gomock.Any(),
					filepath.Join(hostMountDir, taskresourcevolume.TestSourceVolumeHostPath),
					taskresourcevolume.TestFileSystem,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any()).Return(nil)
			},
			setConfirmEBSVolumeIsAttached: func(ctrl *gomock.Controller, mockDiscoveryClient *mock_ebs_discovery.MockEBSDiscovery) {
				var wg sync.WaitGroup
				wg.Add(1)
				mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
					Do(func(deviceName, volumeID string) {
						wg.Done()
					}).
					Return(taskresourcevolume.TestDeviceName, nil)
			},
			expectedNumSavedAttachments: 1,
			attachments: []*apiebs.ResourceAttachment{
				getEBSResourceAttachment(),
			},
		},
		{
			name:         "handle EBS attachment with stopped CSI driver",
			shouldTick:   true,
			numTicks:     2,
			shouldHandle: true,
			setDaemonRunningExpectations: func(ctrl *gomock.Controller, mockTaskEngine *mock_engine.MockTaskEngine, mockDaemonManager *mock_daemonmanager.MockDaemonManager, stateChangeChan chan statechange.Event) {
				daemonManagers := map[string]daemonmanager.DaemonManager{
					md.EbsCsiDriver: mockDaemonManager,
				}
				gomock.InOrder(
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
						Arn:               "arn:aws:ecs:us-east-1:012345678910:task/stopped-task-id",
						KnownStatusUnsafe: status.TaskStopped,
					}),
					mockTaskEngine.EXPECT().GetDaemonTask(md.EbsCsiDriver).Return(&task.Task{
						Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
						KnownStatusUnsafe: status.TaskRunning,
					}),
				)

				mockTaskEngine.EXPECT().GetDaemonManagers().Return(daemonManagers)
				mockDaemonManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
				mockDaemonManager.EXPECT().GetManagedDaemon().Return(md.NewManagedDaemon("name", "tag"))
				mockDaemonManager.EXPECT().CreateDaemonTask().Return(&task.Task{
					Arn:               "arn:aws:ecs:us-east-1:012345678910:task/some-task-id",
					KnownStatusUnsafe: status.TaskCreated,
				}, nil)
				mockTaskEngine.EXPECT().SetDaemonTask(md.EbsCsiDriver, gomock.Any()).Return()
				mockTaskEngine.EXPECT().AddTask(gomock.Any()).Return()
				mockTaskEngine.EXPECT().StateChangeEvents().Return(stateChangeChan)
			},
			setCSIClientExpectations: func(ctrl *gomock.Controller, mockCsiClient *mock_csiclient.MockCSIClient) {
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
			},
			setConfirmEBSVolumeIsAttached: func(ctrl *gomock.Controller, mockDiscoveryClient *mock_ebs_discovery.MockEBSDiscovery) {
				var wg sync.WaitGroup
				wg.Add(1)
				mockDiscoveryClient.EXPECT().ConfirmEBSVolumeIsAttached(taskresourcevolume.TestDeviceName, taskresourcevolume.TestVolumeId).
					Do(func(deviceName, volumeID string) {
						wg.Done()
					}).
					Return(taskresourcevolume.TestDeviceName, nil)
			},
			expectedNumSavedAttachments: 1,
			attachments: []*apiebs.ResourceAttachment{
				getEBSResourceAttachment(),
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			ctx := context.Background()
			taskEngineState := dockerstate.NewTaskEngineState()
			mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
			mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
			mockDaemonManager := mock_daemonmanager.NewMockDaemonManager(mockCtrl)
			mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
			stateChangeChan := make(chan statechange.Event)

			if tc.setDaemonRunningExpectations != nil {
				tc.setDaemonRunningExpectations(mockCtrl, mockTaskEngine, mockDaemonManager, stateChangeChan)
			}
			if tc.setCSIClientExpectations != nil {
				tc.setCSIClientExpectations(mockCtrl, mockCsiClient)
			}
			if tc.setConfirmEBSVolumeIsAttached != nil {
				tc.setConfirmEBSVolumeIsAttached(mockCtrl, mockDiscoveryClient)
			}

			watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)
			for _, ebsAttachment := range tc.attachments {
				err := watcher.HandleEBSResourceAttachment(ebsAttachment)
				if !tc.shouldHandle {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}

			assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), tc.expectedNumSavedAttachments)

			ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(taskresourcevolume.TestVolumeId)
			if !tc.shouldTick {
				assert.False(t, ok)
			} else {
				// Note: There are cases where we need to tick more than once.
				// Ex: Tick 1 -> Create a CSI driver task if it doesn't exist/isn't running, Tick 2 -> Find/Validate/Stage EBS volume
				for i := 0; i < tc.numTicks; i++ {
					watcher.tick()
				}
				assert.True(t, ok)
				assert.True(t, ebsAttachment.IsAttached())
				attachEvent := <-stateChangeChan
				assert.NotNil(t, attachEvent)
				assert.Equal(t, attachEvent.GetEventType(), statechange.AttachmentEvent)
			}
		})
	}
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

// TestHandleEBSAckTimeout tests acknowledging the timeout of a EBS-type resource attachment object saved within the agent state.
// The expected behavior is after the timeout duration, the resource attachment object will be removed from agent state.
// Skip flaky test until debugged
func TestHandleEBSAckTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	taskEngineState := dockerstate.NewTaskEngineState()
	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)
	mockTaskEngine := mock_engine.NewMockTaskEngine(mockCtrl)
	mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)

	ebsAttachment := getEBSResourceAttachment()
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

	ebsAttachment := getEBSResourceAttachment()
	watcher := newTestEBSWatcher(ctx, taskEngineState, mockDiscoveryClient, mockTaskEngine, mockCsiClient)

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

func getEBSAttachmentProperties() map[string]string {
	attachmentProperties := map[string]string{
		apiebs.DeviceNameKey:           taskresourcevolume.TestDeviceName,
		apiebs.VolumeIdKey:             taskresourcevolume.TestVolumeId,
		apiebs.VolumeNameKey:           taskresourcevolume.TestVolumeName,
		apiebs.SourceVolumeHostPathKey: taskresourcevolume.TestSourceVolumeHostPath,
		apiebs.FileSystemKey:           taskresourcevolume.TestFileSystem,
		apiebs.VolumeSizeGibKey:        taskresourcevolume.TestVolumeSizeGib,
	}
	return attachmentProperties
}
