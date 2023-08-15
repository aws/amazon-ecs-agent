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

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

func setupWatcher(ctx context.Context, cancel context.CancelFunc, agentState dockerstate.TaskEngineState,
	ebsChangeEvent chan<- statechange.Event) *EBSWatcher {
	return &EBSWatcher{
		ctx:            ctx,
		cancel:         cancel,
		agentState:     agentState,
		ebsChangeEvent: ebsChangeEvent,
		mailbox:        make(chan func(), 1),
	}
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
