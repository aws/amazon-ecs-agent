// +build unit

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
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestConfig() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	return cfg
}

// TestImagePullRemoveDeadlock tests if there's a deadlock when trying to
// pull an image while image clean up is in progress
func TestImagePullRemoveDeadlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	cfg := defaultTestConfig()
	imageManager := NewImageManager(cfg, client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	sleepContainer := &apicontainer.Container{
		Name:  "sleep",
		Image: "busybox",
	}
	sleepContainerImageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}

	// Cause a fake delay when recording container reference so that the
	// race condition between ImagePullLock and updateLock gets exercised
	// If updateLock precedes ImagePullLock, it can cause a deadlock
	client.EXPECT().InspectImage(sleepContainer.Image).Do(func(image string) {
		time.Sleep(time.Second)
	}).Return(sleepContainerImageInspected, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		ImagePullDeleteLock.Lock()
		defer ImagePullDeleteLock.Unlock()
		err := imageManager.RecordContainerReference(sleepContainer)
		assert.NoError(t, err)
		wg.Done()
	}()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		imageManager.(*dockerImageManager).removeUnusedImages(ctx)
		wg.Done()
	}()
	wg.Wait()
}

func TestAddAndRemoveContainerToImageStateReferenceHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	sourceImageState.AddImageName(container.Image)
	imageManager.(*dockerImageManager).addImageState(sourceImageState)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil)
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, ok := imageManager.(*dockerImageManager).getImageState(imageInspected.ID)
	if !ok {
		t.Error("Error in retrieving existing Image State for the Container")
	}
	if !reflect.DeepEqual(sourceImageState, imageState) {
		t.Error("Mismatch between added and retrieved image state")
	}
	err = imageManager.RemoveContainerReferenceFromImageState(container)
	if err != nil {
		t.Error("Error removing container reference from image state")
	}
	imageState, _ = imageManager.(*dockerImageManager).getImageState(imageInspected.ID)
	if len(imageState.Containers) != 0 {
		t.Error("Error removing container reference from image state")
	}
}

func TestRecordContainerReferenceInspectError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now(),
	}
	sourceImageState.AddImageName(container.Image)
	imageManager.addImageState(sourceImageState)
	client.EXPECT().InspectImage(container.Image).Return(nil, errors.New("error inspecting")).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err == nil {
		t.Error("Expected error in inspecting image while adding container to image state")
	}
}

func TestRecordContainerReferenceWithNoImageName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now(),
	}
	imageManager.addImageState(sourceImageState)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, ok := imageManager.getImageState(imageInspected.ID)
	if !ok {
		t.Error("Error in retrieving existing Image State for the Container")
	}
	for _, imageName := range imageState.Image.Names {
		if imageName != container.Image {
			t.Error("Error while adding image name to image state")
		}
	}
}

func TestAddInvalidContainerReferenceToImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Image: "",
	}
	err := imageManager.RecordContainerReference(container)
	if err == nil {
		t.Error("Expected error adding container reference with no image name to image state")
	}
}

func TestAddContainerReferenceToExistingImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageID := "sha256:qwerty"
	container := &apicontainer.Container{
		Name:    "testContainer",
		Image:   "testContainerImage",
		ImageID: imageID,
	}
	sourceImage := &image.Image{
		ImageID: imageID,
	}
	sourceImageState := &image.ImageState{
		Image: sourceImage,
	}
	sourceImage1 := &image.Image{
		ImageID: "sha256:asdfg",
	}
	sourceImageState1 := &image.ImageState{
		Image: sourceImage1,
	}
	sourceImageState1.AddImageName("testContainerImage")
	imageManager.addImageState(sourceImageState)
	imageManager.addImageState(sourceImageState1)
	if !imageManager.addContainerReferenceToExistingImageState(container) {
		t.Error("Error in adding container to an already existing image state")
	}
	if !reflect.DeepEqual(sourceImageState.Containers[0], container) {
		t.Error("Incorrect container added to an already existing image state")
	}
	if len(sourceImageState1.Image.Names) != 0 {
		t.Error("Error removing existing image name of different ID")
	}
}

func TestAddContainerReferenceToExistingImageStateNoState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	container := &apicontainer.Container{
		Name:    "testContainer",
		Image:   "testContainerImage",
		ImageID: "sha256:qwerty",
	}
	if imageManager.addContainerReferenceToExistingImageState(container) {
		t.Error("Error adding container to an incorrect existing image state")
	}
}

func TestAddContainerReferenceToNewImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageID := "sha256:qwerty"
	var imageSize int64
	imageSize = 18767
	container := &apicontainer.Container{
		Name:    "testContainer",
		Image:   "testContainerImage",
		ImageID: imageID,
	}
	imageManager.addContainerReferenceToNewImageState(container, imageSize)
	_, ok := imageManager.getImageState(imageID)
	if !ok {
		t.Error("Error adding container reference to new image state")
	}
}

func TestAddContainerReferenceToNewImageStateAddedState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageID := "sha256:qwerty"
	var imageSize int64
	imageSize = 18767
	container := &apicontainer.Container{
		Name:    "testContainer",
		Image:   "testContainerImage",
		ImageID: imageID,
	}
	sourceImage := &image.Image{
		ImageID: imageID,
	}
	sourceImageState := &image.ImageState{
		Image: sourceImage,
	}
	sourceImage1 := &image.Image{
		ImageID: "sha256:asdfg",
	}
	sourceImageState1 := &image.ImageState{
		Image: sourceImage1,
	}
	sourceImageState1.AddImageName("testContainerImage")
	imageManager.addImageState(sourceImageState)
	imageManager.addImageState(sourceImageState1)
	imageManager.addContainerReferenceToNewImageState(container, imageSize)
	if !reflect.DeepEqual(sourceImageState.Containers[0], container) {
		t.Error("Incorrect container added to an already existing image state")
	}
	if len(sourceImageState1.Image.Names) != 0 {
		t.Error("Error removing existing image name of different ID")
	}
}

func TestRemoveContainerReferenceFromInvalidImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Image: "myContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RemoveContainerReferenceFromImageState(container)
	if err == nil {
		t.Error("Expected error while adding container to an invalid image state")
	}
}

func TestRemoveInvalidContainerReferenceFromImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Image: "",
	}
	err := imageManager.RemoveContainerReferenceFromImageState(container)
	if err == nil {
		t.Error("Expected error removing container reference with no image name from image state")
	}
}

func TestRemoveContainerReferenceFromImageStateInspectError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Image: "myContainerImage",
	}
	client.EXPECT().InspectImage(container.Image).Return(nil, errors.New("error inspecting")).AnyTimes()
	err := imageManager.RemoveContainerReferenceFromImageState(container)
	if err == nil {
		t.Error("Expected error in inspecting image while adding container to image state")
	}
}

func TestRemoveContainerReferenceFromImageStateWithNoReference(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now(),
	}
	imageManager.addImageState(sourceImageState)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RemoveContainerReferenceFromImageState(container)
	if err == nil {
		t.Error("Expected error removing non-existing container reference from image state")
	}
}

func TestGetCandidateImagesForDeletionImageNoImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageStates := imageManager.getCandidateImagesForDeletion()

	if imageStates != nil {
		t.Error("Expected no image state to be returned for deletion")
	}
}

func TestGetCandidateImagesForDeletionImageJustPulled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	sourceImage := &image.Image{}
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now(),
	}
	imageManager.addImageState(sourceImageState)
	imageStates := imageManager.getCandidateImagesForDeletion()
	if len(imageStates) > 0 {
		t.Error("Expected no image state to be returned for deletion")
	}
}

func TestGetCandidateImagesForDeletionImageHasContainerReference(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	imageManager.addImageState(sourceImageState)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageStates := imageManager.getCandidateImagesForDeletion()
	if len(imageStates) > 0 {
		t.Error("Expected no image state to be returned for deletion")
	}
}

func TestGetCandidateImagesForDeletionImageHasMoreContainerReferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	container2 := &apicontainer.Container{
		Name:  "testContainer2",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	sourceImageState := &image.ImageState{
		Image:    sourceImage,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	imageManager.addImageState(sourceImageState)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err = imageManager.RecordContainerReference(container2)
	if err != nil {
		t.Error("Error in adding container2 to an existing image state")
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err = imageManager.RemoveContainerReferenceFromImageState(container)
	if err != nil {
		t.Error("Error removing container reference from image state")
	}
	imageStates := imageManager.getCandidateImagesForDeletion()
	if len(imageStates) > 0 {
		t.Error("Expected no image state to be returned for deletion")
	}
}

func TestImageCleanupExclusionListWithSingleName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	sourceImageA := &image.Image{
		ImageID: "sha256:qwerty1",
		Names:   []string{"a"},
	}
	sourceImageB := &image.Image{
		ImageID: "sha256:qwerty2",
		Names:   []string{"b"},
	}
	sourceImageC := &image.Image{
		ImageID: "sha256:qwerty3",
		Names:   []string{"c"},
	}
	ImageStateA := &image.ImageState{
		Image:    sourceImageA,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	ImageStateB := &image.ImageState{
		Image:    sourceImageB,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	ImageStateC := &image.ImageState{
		Image:    sourceImageC,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	imageManager := &dockerImageManager{
		client:                    client,
		state:                     dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion:  config.DefaultImageDeletionAge,
		numImagesToDelete:         config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval:  config.DefaultImageCleanupTimeInterval,
		imageCleanupExclusionList: []string{"a", "c"},
		imageStatesConsideredForDeletion: map[string]*image.ImageState{
			"sha256:qwerty2": ImageStateB,
		},
	}
	var testImageStates = []*image.ImageState{ImageStateA, ImageStateB, ImageStateC}
	var testResult = imageManager.imagesConsiderForDeletion(testImageStates)

	assert.Equal(t, 1, len(testResult), "Expected 1 image state to be returned for deletion")
	if !reflect.DeepEqual(imageManager.imageStatesConsideredForDeletion, testResult) {
		t.Error("Incorrect image return from  getCandidateImagesForDeletionHelper function")
	}
}

func TestImageCleanupExclusionListWithMultipleNames(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	sourceImageA := &image.Image{
		ImageID: "sha256:qwerty1",
		Names:   []string{"a", "b", "c"},
	}
	sourceImageB := &image.Image{
		ImageID: "sha256:qwerty2",
		Names:   []string{"d", "e", "f"},
	}
	sourceImageC := &image.Image{
		ImageID: "sha256:qwerty3",
		Names:   []string{"g", "h", "i"},
	}
	sourceImageD := &image.Image{
		ImageID: "sha256:qwerty4",
		Names:   []string{"x", "y", "z"},
	}
	ImageStateA := &image.ImageState{
		Image:    sourceImageA,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	ImageStateB := &image.ImageState{
		Image:    sourceImageB,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	ImageStateC := &image.ImageState{
		Image:    sourceImageC,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	ImageStateD := &image.ImageState{
		Image:    sourceImageD,
		PulledAt: time.Now().AddDate(0, -2, 0),
	}
	imageManager := &dockerImageManager{
		client:                    client,
		state:                     dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion:  config.DefaultImageDeletionAge,
		numImagesToDelete:         config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval:  config.DefaultImageCleanupTimeInterval,
		imageCleanupExclusionList: []string{"a", "d", "g"},
		imageStatesConsideredForDeletion: map[string]*image.ImageState{
			"sha256:qwerty4": ImageStateD,
		},
	}
	var testImageStates = []*image.ImageState{ImageStateA, ImageStateB, ImageStateC, ImageStateD}
	var testResult = imageManager.imagesConsiderForDeletion(testImageStates)

	assert.Equal(t, 1, len(testResult), "Expected 1 image state to be returned for deletion")
	if !reflect.DeepEqual(imageManager.imageStatesConsideredForDeletion, testResult) {
		t.Error("Incorrect image return from  getCandidateImagesForDeletionHelper function")
	}
}

func TestGetLeastRecentlyUsedImages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := NewImageManager(defaultTestConfig(), client, dockerstate.NewTaskEngineState())

	imageStateA := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -5, 0),
	}
	imageStateB := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -3, 0),
	}
	imageStateC := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -2, 0),
	}
	imageStateD := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -6, 0),
	}
	imageStateE := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -4, 0),
	}
	imageStateF := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -1, 0),
	}

	candidateImagesForDeletion := []*image.ImageState{
		imageStateA, imageStateB, imageStateC, imageStateD, imageStateE, imageStateF,
	}
	expectedLeastRecentlyUsedImages := []*image.ImageState{
		imageStateD, imageStateA, imageStateE, imageStateB, imageStateC,
	}
	leastRecentlyUsedImage := imageManager.(*dockerImageManager).getLeastRecentlyUsedImage(candidateImagesForDeletion)
	if !reflect.DeepEqual(leastRecentlyUsedImage, expectedLeastRecentlyUsedImages[0]) {
		t.Error("Incorrect order of least recently used images")
	}
}

func TestGetLeastRecentlyUsedImagesLessThanFive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageStateA := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -5, 0),
	}
	imageStateB := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -3, 0),
	}
	imageStateC := &image.ImageState{
		LastUsedAt: time.Now().AddDate(0, -2, 0),
	}
	candidateImagesForDeletion := []*image.ImageState{
		imageStateA, imageStateB, imageStateC,
	}
	expectedLeastRecentlyUsedImages := []*image.ImageState{
		imageStateA, imageStateB, imageStateC,
	}
	leastRecentlyUsedImage := imageManager.getLeastRecentlyUsedImage(candidateImagesForDeletion)
	if !reflect.DeepEqual(leastRecentlyUsedImage, expectedLeastRecentlyUsedImages[0]) {
		t.Error("Incorrect order of least recently used images")
	}
}

func TestRemoveAlreadyExistingImageNameWithDifferentID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil)
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	container1 := &apicontainer.Container{
		Name:  "testContainer1",
		Image: "testContainerImage",
	}
	imageInspected1 := &docker.Image{
		ID: "sha256:asdfg",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected1, nil)
	err = imageManager.RecordContainerReference(container1)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, ok := imageManager.getImageState(imageInspected.ID)
	if !ok {
		t.Error("Error in retrieving existing Image State for the Container")
	}
	if len(imageState.Image.Names) != 0 {
		t.Error("Error in removing already existing image name with different ID")
	}
}

func TestImageCleanupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: 1 * time.Millisecond,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}

	err = imageManager.RemoveContainerReferenceFromImageState(container)
	if err != nil {
		t.Error("Error removing container reference from image state")
	}

	imageState, _ := imageManager.getImageState(imageInspected.ID)
	imageState.PulledAt = time.Now().AddDate(0, -2, 0)
	imageState.LastUsedAt = time.Now().AddDate(0, -2, 0)
	imageState.AddImageName("anotherImage")

	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(nil)
	client.EXPECT().RemoveImage(gomock.Any(), "anotherImage", dockerclient.RemoveImageTimeout).Return(nil)
	parent := context.Background()
	ctx, cancel := context.WithCancel(parent)
	go imageManager.performPeriodicImageCleanup(ctx, 2*time.Millisecond)
	time.Sleep(1 * time.Second)
	cancel()
	if imageState.GetImageNamesCount() != 0 {
		t.Error("Error removing image name from state after the image is removed")
	}
	if imageManager.GetImageStatesCount() != 0 {
		t.Error("Error removing image state after the image is removed")
	}
}

func TestImageCleanupCannotRemoveImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}

	err = imageManager.RemoveContainerReferenceFromImageState(container)
	if err != nil {
		t.Error("Error removing container reference from image state")
	}

	imageState, _ := imageManager.getImageState(imageInspected.ID)
	imageState.PulledAt = time.Now().AddDate(0, -2, 0)
	imageState.LastUsedAt = time.Now().AddDate(0, -2, 0)

	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(
		errors.New("error removing image")).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.removeUnusedImages(ctx)
	if len(imageState.Image.Names) == 0 {
		t.Error("Error: image name should not be removed")
	}
	if len(imageManager.imageStates) == 0 {
		t.Error("Error: image state should not be removed")
	}
}

func TestImageCleanupRemoveImageById(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}

	err = imageManager.RemoveContainerReferenceFromImageState(container)
	if err != nil {
		t.Error("Error removing container reference from image state")
	}

	imageState, _ := imageManager.getImageState(imageInspected.ID)
	imageState.RemoveImageName(container.Image)
	imageState.PulledAt = time.Now().AddDate(0, -2, 0)
	imageState.LastUsedAt = time.Now().AddDate(0, -2, 0)

	client.EXPECT().RemoveImage(gomock.Any(), sourceImage.ImageID, dockerclient.RemoveImageTimeout).Return(nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.removeUnusedImages(ctx)
	if len(imageManager.imageStates) != 0 {
		t.Error("Error removing image state after the image is removed")
	}
}

func TestDeleteImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, _ := imageManager.getImageState(imageInspected.ID)
	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.deleteImage(ctx, container.Image, imageState)
	if len(imageState.Image.Names) != 0 {
		t.Error("Error removing Image name from image state")
	}
	if len(imageManager.getAllImageStates()) != 0 {
		t.Error("Error removing image state from image manager after deletion")
	}
}

func TestDeleteImageNotFoundError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, _ := imageManager.getImageState(imageInspected.ID)
	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(
		errors.New("no such image"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.deleteImage(ctx, container.Image, imageState)
	if len(imageState.Image.Names) != 0 {
		t.Error("Error removing Image name from image state")
	}
	if len(imageManager.getAllImageStates()) != 0 {
		t.Error("Error removing image state from image manager")
	}
}

func TestDeleteImageOtherRemoveImageErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	imageState, _ := imageManager.getImageState(imageInspected.ID)
	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(
		errors.New("container for this image exists"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.deleteImage(ctx, container.Image, imageState)
	if len(imageState.Image.Names) == 0 {
		t.Error("Incorrectly removed Image name from image state")
	}
	if len(imageManager.getAllImageStates()) == 0 {
		t.Error("Incorrecting removed image state from image manager before deletion")
	}
}

func TestDeleteImageIDNull(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.deleteImage(ctx, "", nil)
}

func TestRemoveLeastRecentlyUsedImageNoImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := imageManager.removeLeastRecentlyUsedImage(ctx)
	if err == nil {
		t.Error("Expected Error for no LRU image to remove")
	}
}

func TestRemoveUnusedImagesNoImages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.removeUnusedImages(ctx)
}

func TestGetImageStateFromImageName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	_, ok := imageManager.GetImageStateFromImageName(container.Image)
	if !ok {
		t.Error("Error retrieving image state by image name")
	}
}

func TestGetImageStateFromImageNameNoImageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	imageManager := &dockerImageManager{client: client, state: dockerstate.NewTaskEngineState()}
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	_, ok := imageManager.GetImageStateFromImageName("noSuchImage")
	if ok {
		t.Error("Incorrect image state retrieved by image name")
	}
}

// TestConcurrentRemoveUnusedImages checks for concurrent map writes
// in the imageManager
func TestConcurrentRemoveUnusedImages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	imageManager := &dockerImageManager{
		client:                   client,
		state:                    dockerstate.NewTaskEngineState(),
		minimumAgeBeforeDeletion: config.DefaultImageDeletionAge,
		numImagesToDelete:        config.DefaultNumImagesToDeletePerCycle,
		imageCleanupTimeInterval: config.DefaultImageCleanupTimeInterval,
	}

	imageManager.SetSaver(statemanager.NewNoopStateManager())
	container := &apicontainer.Container{
		Name:  "testContainer",
		Image: "testContainerImage",
	}
	sourceImage := &image.Image{
		ImageID: "sha256:qwerty",
	}
	sourceImage.Names = append(sourceImage.Names, container.Image)
	imageInspected := &docker.Image{
		ID: "sha256:qwerty",
	}
	client.EXPECT().InspectImage(container.Image).Return(imageInspected, nil).AnyTimes()
	err := imageManager.RecordContainerReference(container)
	if err != nil {
		t.Error("Error in adding container to an existing image state")
	}
	require.Equal(t, 1, len(imageManager.imageStates))

	// Remove container reference from image state to trigger cleanup
	err = imageManager.RemoveContainerReferenceFromImageState(container)
	assert.NoError(t, err)

	imageState, _ := imageManager.getImageState(imageInspected.ID)
	imageState.PulledAt = time.Now().AddDate(0, -2, 0)
	imageState.LastUsedAt = time.Now().AddDate(0, -2, 0)

	client.EXPECT().RemoveImage(gomock.Any(), container.Image, dockerclient.RemoveImageTimeout).Return(nil)
	require.Equal(t, 1, len(imageManager.imageStates))

	// We create 1000 goroutines and then perform a channel close
	// to simulate the concurrent map write problem
	numRoutines := 1000
	var waitGroup sync.WaitGroup
	waitGroup.Add(numRoutines)

	ok := make(chan bool)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	for i := 0; i < numRoutines; i++ {
		go func() {
			<-ok
			imageManager.removeUnusedImages(ctx)
			waitGroup.Done()
		}()
	}

	close(ok)
	waitGroup.Wait()
	require.Equal(t, 0, len(imageManager.imageStates))
}

func TestImageCleanupProcessNotStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	cfg := defaultTestConfig()
	cfg.ImagePullBehavior = config.ImagePullPreferCachedBehavior
	imageManager := NewImageManager(cfg, client, dockerstate.NewTaskEngineState())
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	imageManager.StartImageCleanupProcess(ctx)
	// Nothing should happen.
}
