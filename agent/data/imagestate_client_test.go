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

package data

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/engine/image"

	"github.com/stretchr/testify/assert"
)

const (
	testImageId    = "test-id"
	testImageId2   = "test-id2"
	testImageName  = "testName"
	testImageName2 = "testName2"
)

func TestManageImageStates(t *testing.T) {
	testClient := newTestClient(t)

	testImageState := &image.ImageState{
		Image: &image.Image{
			ImageID: testImageId,
			Names:   []string{testImageName},
		},
		PullSucceeded: false,
	}

	assert.NoError(t, testClient.SaveImageState(testImageState))
	testImageState.SetPullSucceeded(true)
	assert.NoError(t, testClient.SaveImageState(testImageState))
	res, err := testClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, true, res[0].PullSucceeded)
	assert.Equal(t, testImageId, res[0].Image.ImageID)
	assert.Equal(t, testImageName, res[0].Image.Names[0])

	testImageState2 := &image.ImageState{
		Image: &image.Image{
			ImageID: testImageId2,
			Names:   []string{testImageName2},
		},
		PullSucceeded: false,
	}
	assert.NoError(t, testClient.SaveImageState(testImageState2))
	res, err = testClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, res, 2)

	assert.NoError(t, testClient.DeleteImageState(testImageId))
	assert.NoError(t, testClient.DeleteImageState(testImageId2))
	res, err = testClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, res, 0)
}
