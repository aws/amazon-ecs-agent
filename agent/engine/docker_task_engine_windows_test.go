// +build windows,unit

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
package engine

import (
	"context"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/golang/mock/gomock"
)

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	task := &apitask.Task{}

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &defaultConfig,
		ctx:   ctx,
	}

	gomock.InOrder(
		mockState.EXPECT().RemoveTask(task),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}
