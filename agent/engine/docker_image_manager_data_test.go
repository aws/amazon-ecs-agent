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

package engine

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"

	"github.com/stretchr/testify/assert"
)

/*
This file contains unit tests that specifically check the data persisting behavior of the image manager
*/

const (
	testImageID   = "test-id"
	testImageID2  = "test-id2"
	testImageName = "test-name"
)

var (
	testImageStateData = &image.ImageState{
		Image: &image.Image{
			ImageID: testImageID,
		},
	}
	testContainerData = &apicontainer.Container{
		Name:    testContainerName,
		ImageID: testImageID,
	}
)

func TestAddImageStateSaveData(t *testing.T) {
	dataClient := newTestDataClient(t)

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}

	imageManager.addImageState(testImageStateData)
	imageStates, err := dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 1)
}

func TestAddContainerReferenceToNewImageStateSaveData(t *testing.T) {
	dataClient := newTestDataClient(t)

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}

	imageManager.addContainerReferenceToNewImageState(testContainerData, 0)
	imageStates, err := dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 1)
}

func TestAddContainerReferenceToExistingImageStateSaveData(t *testing.T) {
	dataClient := newTestDataClient(t)

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}
	imageManager.imageStates = append(imageManager.imageStates, testImageStateData)
	imageManager.addContainerReferenceToExistingImageState(testContainerData)
	imageStates, err := dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 1)
}

func TestRemoveExistingImageNameOfDifferentID(t *testing.T) {
	dataClient := newTestDataClient(t)

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}

	testImageStateData.Image.Names = []string{testImageName}
	imageManager.addImageState(testImageStateData)
	imageStates, err := dataClient.GetImageStates()
	assert.Len(t, imageStates[0].Image.Names, 1)

	imageManager.removeExistingImageNameOfDifferentID(testImageName, testImageID2)
	imageStates, err = dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 1)
	assert.Len(t, imageStates[0].Image.Names, 0)
}

func TestRemoveImageStateSaveData(t *testing.T) {
	dataClient := newTestDataClient(t)

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}

	imageManager.addImageState(testImageStateData)
	imageManager.removeImageStateData(testImageID)
	imageStates, err := dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 0)
}
