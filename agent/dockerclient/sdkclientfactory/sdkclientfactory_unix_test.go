//go:build unit && !windows
// +build unit,!windows

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

package sdkclientfactory

import (
	"context"
	"testing"

	mock_sdkclient "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestFindClientAPIVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl := gomock.NewController(t)
	mockClient := mock_sdkclient.NewMockClient(ctrl)
	factory := NewFactory(ctx, expectedEndpoint)

	for _, version := range getAgentSupportedDockerVersions() {
		mockClient.EXPECT().ClientVersion().Return(string(version))
		assert.Equal(t, version, factory.FindClientAPIVersion(mockClient))
	}
}
