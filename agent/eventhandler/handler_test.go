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

package eventhandler

import (
	"context"
	"sync"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestHandleEngineEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_api.NewMockECSClient(ctrl)

	ctx, cancel := context.WithCancel(context.Background())
	taskHandler := NewTaskHandler(ctx, data.NewNoopClient(), dockerstate.NewTaskEngineState(), client)
	attachmentHandler := NewAttachmentEventHandler(ctx, data.NewNoopClient(), client)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	contEvent1 := containerEvent(taskARN)
	contEvent2 := containerEvent(taskARN)
	taskEvent := taskEvent(taskARN)
	attachmentEvent := attachmentEvent("attachmentARN")

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, 2, len(change.Containers))
		assert.Equal(t, taskARN, change.Containers[0].TaskArn)
		assert.Equal(t, taskARN, change.Containers[1].TaskArn)
		wg.Done()
	})

	client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Do(func(change api.AttachmentStateChange) {
		assert.NotNil(t, change.Attachment)
		assert.Equal(t, "attachmentARN", change.Attachment.AttachmentARN)
		wg.Done()
	})

	handleEngineEvent(contEvent1, client, taskHandler, attachmentHandler)
	handleEngineEvent(contEvent2, client, taskHandler, attachmentHandler)
	handleEngineEvent(taskEvent, client, taskHandler, attachmentHandler)
	handleEngineEvent(attachmentEvent, client, taskHandler, attachmentHandler)

	wg.Wait()
}
