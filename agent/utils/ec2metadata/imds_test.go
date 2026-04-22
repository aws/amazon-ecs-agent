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

package ec2metadata

import (
	"errors"
	"testing"

	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestIsIMDSAvailable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "available",
			expected: true,
		},
		{
			name:     "unavailable",
			err:      errors.New("request timeout"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mockClient.EXPECT().GetMetadata("instance-id").Return("", tc.err)
			assert.Equal(t, tc.expected, IsIMDSAvailable(mockClient))
		})
	}
}
