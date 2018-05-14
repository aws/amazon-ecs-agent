// +build unit

// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v2

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/stats/mock"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestTaskStatsResponseSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	dockerStats := &docker.Stats{NumProcs: 2}
	containerMap := map[string]*apicontainer.DockerContainer{
		containerName: &apicontainer.DockerContainer{
			DockerID: containerID,
		},
	}
	gomock.InOrder(
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, nil),
	)

	resp, err := NewTaskStatsResponse(taskARN, state, statsEngine)
	assert.NoError(t, err)
	containerStats, ok := resp[containerID]
	assert.True(t, ok)
	assert.Equal(t, dockerStats.NumProcs, containerStats.NumProcs)
}

func TestTaskStatsResponseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	state.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)
	_, err := NewTaskStatsResponse(taskARN, state, statsEngine)
	assert.Error(t, err)
}
