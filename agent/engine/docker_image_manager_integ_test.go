// +build integration
// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// The DockerTaskEngine is an abstraction over the DockerGoClient so that
// it does not have to know about tasks, only containers

package engine

import (
	"container/list"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	docker "github.com/fsouza/go-dockerclient"
)

const (
	imageRemovalTimeout       = 30 * time.Second
	taskCleanupTimeoutSeconds = 30
)

// Deletion of images in the order of LRU time: Happy path
//  a. This includes starting up agent, pull images, start containers,
//    account them in image manager,  stop containers, remove containers, account this in image manager,
//  b. Simulate the pulled time (so that it passes the minimum age criteria
//    for getting chosen for deletion )
//  c. Start image cleanup , ensure that ONLY the top 2 eligible LRU images
//    are removed from the instance,  and those deleted images’ image states are removed from image manager.
//  d. Ensure images that do not pass the ‘minimumAgeForDeletion’ criteria are not removed.
//  e. Image has not passed the ‘hasNoAssociatedContainers’ criteria.
//  f. Ensure that that if not eligible, image is not deleted from the instance and image reference in ImageManager is not removed.
func TestIntegImageCleanupHappyCase(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 5 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 1 * time.Second
	cfg.NumImagesToDeletePerCycle = 2
	// start agent
	taskEngine, done, _ := setup(cfg, t)

	imageManager := taskEngine.(*DockerTaskEngine).imageManager.(*dockerImageManager)
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	defer func() {
		done()
		// Force cleanup all test images and containers
		cleanupImagesHappy(imageManager)
	}()

	taskEvents, containerEvents := taskEngine.TaskEvents()

	defer discardEvents(containerEvents)()

	// Create test Task
	taskName := "imgClean"
	testTask := createImageCleanupHappyTestTask(taskName)

	go taskEngine.AddTask(testTask)

	// Verify that Task is running
	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	imageState1 := imageManager.GetImageStateFromImageName(test1Image1Name)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", test1Image1Name)
	} else {
		t.Logf("Found image state for %s", test1Image1Name)
	}
	imageState2 := imageManager.GetImageStateFromImageName(test1Image2Name)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", test1Image2Name)
	} else {
		t.Logf("Found image state for %s", test1Image2Name)
	}
	imageState3 := imageManager.GetImageStateFromImageName(test1Image3Name)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", test1Image3Name)
	} else {
		t.Logf("Found image state for %s", test1Image3Name)
	}

	imageState1ImageID := imageState1.Image.ImageID
	imageState2ImageID := imageState2.Image.ImageID
	imageState3ImageID := imageState3.Image.ImageID

	// Set the ImageState.LastUsedAt to a value far in the past to ensure the test images are deleted.
	// This will make these test images the LRU images.
	imageState1.LastUsedAt = imageState1.LastUsedAt.Add(-99995 * time.Hour)
	imageState2.LastUsedAt = imageState2.LastUsedAt.Add(-99994 * time.Hour)
	imageState3.LastUsedAt = imageState3.LastUsedAt.Add(-99993 * time.Hour)

	// Verify Task is stopped.
	verifyTaskIsStopped(taskEvents, testTask)

	// Allow Task cleanup to occur
	time.Sleep(5 * time.Second)

	// Verify Task is cleaned up
	err = verifyTaskIsCleanedUp(taskName, taskEngine)
	if err != nil {
		t.Fatal(err)
	}

	// Call Image removal
	imageManager.removeUnusedImages()

	// Verify top 2 LRU images are deleted from image manager
	err = verifyImagesAreRemoved(imageManager, imageState1ImageID, imageState2ImageID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify 3rd LRU image is not removed
	err = verifyImagesAreNotRemoved(imageManager, imageState3ImageID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify top 2 LRU images are removed from docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState1ImageID)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState2ImageID)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}

	// Verify 3rd LRU image has not been removed from Docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState3ImageID)
	if err != nil {
		t.Fatalf("Image should not have been removed from Docker")
	}
}

// Test that images not falling in the image deletion eligibility criteria are not removed:
//  a. Ensure images that do not pass the ‘minimumAgeForDeletion’ criteria are not removed.
//  b. Image has not passed the ‘hasNoAssociatedContainers’ criteria.
//  c. Ensure that the image is not deleted from the instance and image reference in ImageManager is not removed.
func TestIntegImageCleanupThreshold(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 15 * time.Minute
	// Set to delete three images, but in this test we expect only two images to be removed
	cfg.NumImagesToDeletePerCycle = 3
	// start agent
	taskEngine, done, _ := setup(cfg, t)

	imageManager := taskEngine.(*DockerTaskEngine).imageManager.(*dockerImageManager)
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	defer func() {
		done()
		// Force cleanup all test images and containers
		cleanupImagesThreshold(imageManager)
	}()

	taskEvents, containerEvents := taskEngine.TaskEvents()
	defer discardEvents(containerEvents)()

	// Create test Task
	taskName := "imgClean"
	testTask := createImageCleanupThresholdTestTask(taskName)

	// Start Task
	go taskEngine.AddTask(testTask)

	// Verify that Task is running
	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	imageState1 := imageManager.GetImageStateFromImageName(test2Image1Name)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", test2Image1Name)
	} else {
		t.Logf("Found image state for %s", test2Image1Name)
	}
	imageState2 := imageManager.GetImageStateFromImageName(test2Image2Name)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", test2Image2Name)
	} else {
		t.Logf("Found image state for %s", test2Image2Name)
	}
	imageState3 := imageManager.GetImageStateFromImageName(test2Image3Name)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", test2Image3Name)
	} else {
		t.Logf("Found image state for %s", test2Image3Name)
	}

	imageState1ImageID := imageState1.Image.ImageID
	imageState2ImageID := imageState2.Image.ImageID
	imageState3ImageID := imageState3.Image.ImageID

	// Set the ImageState.LastUsedAt to a value far in the past to ensure the test images are deleted.
	// This will make these the LRU images so they are deleted.
	imageState1.LastUsedAt = imageState1.LastUsedAt.Add(-99995 * time.Hour)
	imageState2.LastUsedAt = imageState2.LastUsedAt.Add(-99994 * time.Hour)
	imageState3.LastUsedAt = imageState3.LastUsedAt.Add(-99993 * time.Hour)

	// Set two containers to have pull time > threshold
	imageState1.PulledAt = imageState1.PulledAt.Add(-20 * time.Minute)
	imageState2.PulledAt = imageState2.PulledAt.Add(-10 * time.Minute)
	imageState3.PulledAt = imageState3.PulledAt.Add(-25 * time.Minute)

	// Verify Task is stopped
	verifyTaskIsStopped(taskEvents, testTask)

	// Allow Task cleanup to occur
	time.Sleep(2 * time.Second)

	// Verify Task is cleaned up
	err = verifyTaskIsCleanedUp(taskName, taskEngine)
	if err != nil {
		t.Fatal(err)
	}

	// Call Image removal
	imageManager.removeUnusedImages()

	// Verify Image1 & Image3 are removed from ImageManager as they are beyond the minimumAge threshold
	err = verifyImagesAreRemoved(imageManager, imageState1ImageID, imageState3ImageID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Image2 is not removed, below threshold for minimumAge
	err = verifyImagesAreNotRemoved(imageManager, imageState2ImageID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Image1 & Image3 are removed from docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState1ImageID)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState3ImageID)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}

	// Verify Image2 has not been removed from Docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState2ImageID)
	if err != nil {
		t.Fatalf("Image should not have been removed from Docker")
	}
}

func createImageCleanupHappyTestTask(taskName string) *api.Task {
	return &api.Task{
		Arn:           taskName,
		Family:        taskName,
		Version:       "1",
		DesiredStatus: api.TaskRunning,
		Containers: []*api.Container{
			&api.Container{
				Name:          "test1",
				Image:         test1Image1Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
			&api.Container{
				Name:          "test2",
				Image:         test1Image2Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
			&api.Container{
				Name:          "test3",
				Image:         test1Image3Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
		},
	}
}

func createImageCleanupThresholdTestTask(taskName string) *api.Task {
	return &api.Task{
		Arn:           taskName,
		Family:        taskName,
		Version:       "1",
		DesiredStatus: api.TaskRunning,
		Containers: []*api.Container{
			&api.Container{
				Name:          "test1",
				Image:         test2Image1Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
			&api.Container{
				Name:          "test2",
				Image:         test2Image2Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
			&api.Container{
				Name:          "test3",
				Image:         test2Image3Name,
				Essential:     false,
				DesiredStatus: api.ContainerRunning,
				Cpu:           10,
				Memory:        10,
			},
		},
	}
}

func verifyTaskIsCleanedUp(taskName string, taskEngine TaskEngine) error {
	for i := 0; i < taskCleanupTimeoutSeconds; i++ {
		_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(taskName)
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
		if i == (taskCleanupTimeoutSeconds - 1) {
			return errors.New("Expected Task to have been swept but was not")
		}
	}
	return nil
}

func verifyImagesAreRemoved(imageManager *dockerImageManager, imageIDs ...string) error {
	imagesNotRemovedList := list.New()
	for _, imageID := range imageIDs {
		_, ok := imageManager.getImageState(imageID)
		if ok {
			imagesNotRemovedList.PushFront(imageID)
		}
	}
	if imagesNotRemovedList.Len() > 0 {
		return fmt.Errorf("Image states still exist for: %v", imagesNotRemovedList)
	}
	return nil
}

func verifyImagesAreNotRemoved(imageManager *dockerImageManager, imageIDs ...string) error {
	imagesRemovedList := list.New()
	for _, imageID := range imageIDs {
		_, ok := imageManager.getImageState(imageID)
		if !ok {
			imagesRemovedList.PushFront(imageID)
		}
	}
	if imagesRemovedList.Len() > 0 {
		return fmt.Errorf("Could not find images: %v in ImageManager", imagesRemovedList)
	}
	return nil
}

func cleanupImagesHappy(imageManager *dockerImageManager) {
	imageManager.client.RemoveContainer("test1", removeContainerTimeout)
	imageManager.client.RemoveContainer("test2", removeContainerTimeout)
	imageManager.client.RemoveContainer("test3", removeContainerTimeout)
	imageManager.client.RemoveImage(test1Image1Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(test1Image2Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(test1Image3Name, imageRemovalTimeout)
}

func cleanupImagesThreshold(imageManager *dockerImageManager) {
	imageManager.client.RemoveContainer("test1", removeContainerTimeout)
	imageManager.client.RemoveContainer("test2", removeContainerTimeout)
	imageManager.client.RemoveContainer("test3", removeContainerTimeout)
	imageManager.client.RemoveImage(test2Image1Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(test2Image2Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(test2Image3Name, imageRemovalTimeout)
}
