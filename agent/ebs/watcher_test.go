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
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	mock_ebs_discovery "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource/mocks"

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

// Start the given actor in a new goroutine. The actor must be initialized with
// the group's context.
func (tg *TestGroup) Start(watcher *EBSWatcher) {
	tg.wg.Add(1)
	go func() {
		defer tg.wg.Done()
		watcher.Start()
	}()
}

// Cancel all actors started by Start() by calling the associated context and wait its completion.
func (tg *TestGroup) Cancel() {
	// Stop the actors.
	tg.cancel()

	// Make sure all actors have been stopped.
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

	// mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	mockDiscoveryClient := mock_ebs_discovery.NewMockEBSDiscovery(mockCtrl)

	// testGroup.Start(watcher)

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

	// err := watcher.addEBSAttachmentToState(ebsAttachment)
	// assert.NoError(t, err)
	// assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	// ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	// assert.True(t, ok)

	// err := watcher.HandleResourceAttachment(ebsAttachment)

	watcher := setupWatcher(ctx, cancel, taskEngineState, eventChannel, mockDiscoveryClient)
	testGroup.Start(watcher)

	watcher.HandleResourceAttachment(ebsAttachment)
	// assert.NoError(t, err)

	wg.Wait()

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)
	ebsAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).GetEBSByVolumeId(volumeID)
	assert.True(t, ok)
}

// func TestHandleEBSAttachment(t *testing.T) {
// 	mockCtrl := gomock.NewController(t)
// 	defer mockCtrl.Finish()

// 	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
// 	eventChannel := make(chan statechange.Event)
// 	ctx := context.TODO()

// 	gomock.InOrder(
// 		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
// 			AttachmentInfo: attachmentinfo.AttachmentInfo{
// 				ExpiresAt: time.Now().Add(expirationTimeAddition),
// 			},
// 		}, true),
// 	)

// 	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel)
// }

// func TestEBSAckTimeout(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	taskEngineState := dockerstate.NewTaskEngineState()
// 	dataClient := newTestDataClient(t)

// 	testAttachmentProperties := map[string]string{
// 		apira.ResourceTypeName:    apira.ElasticBlockStorage,
// 		apira.RequestedSizeName:   "5",
// 		apira.VolumeSizeInGiBName: "7",
// 		apira.DeviceName:          "/host/dev/nvme1n1",
// 		apira.VolumeIdName:        "vol-123",
// 		apira.FileSystemTypeName:  "testXFS",
// 	}
// 	expiresAt := time.Now().Add(time.Millisecond * 1000)
// 	ebsAttachment := &apira.ResourceAttachment{
// 		AttachmentInfo: attachmentinfo.AttachmentInfo{
// 			AttachmentARN:        "dummy-arn",
// 			Status:               status.AttachmentNone,
// 			ExpiresAt:            expiresAt,
// 			AttachStatusSent:     false,
// 			ClusterARN:           "dummy-cluster-arn",
// 			ContainerInstanceARN: "dummy-container-instance-arn",
// 		},
// 		AttachmentProperties: testAttachmentProperties,
// 	}
// 	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel)

// 	err := watcher.addEBSAttachmentToState(ebsAttachment)
// 	assert.NoError(t, err)
// 	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 1)

// 	for {
// 		time.Sleep(time.Millisecond * 1000)
// 		assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).GetAllEBSAttachments(), 0)
// 		break
// 	}
// }

// func TestHandleResourceAttachment(t *testing.T) {

// }
