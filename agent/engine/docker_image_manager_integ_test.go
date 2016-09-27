// +build integration !unit
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

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	docker "github.com/fsouza/go-dockerclient"
)

const (
	imageRemovalTimeout       = 30 * time.Second
	taskCleanupTimeoutSeconds = 30

	testImage1Name = "127.0.0.1:51670/amazon/image-cleanup-test-image1:latest"
	testImage2Name = "127.0.0.1:51670/amazon/image-cleanup-test-image2:latest"
	testImage3Name = "127.0.0.1:51670/amazon/image-cleanup-test-image3:latest"
)

const credentialsIdIntegTest = "credsid"

func defaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	return cfg
}

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
		cleanupImages(imageManager)
	}()

	taskEvents, containerEvents := taskEngine.TaskEvents()

	defer discardEvents(containerEvents)()

	// Create test Task
	taskName := "imgClean"
	testTask := createImageCleanupTestTask(taskName)

	go taskEngine.AddTask(testTask)

	// Verify that Task is running
	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	imageState1 := imageManager.GetImageStateFromImageName(testImage1Name)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", testImage1Name)
	} else {
		t.Logf("Found image state for %s", testImage1Name)
	}
	imageState2 := imageManager.GetImageStateFromImageName(testImage2Name)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", testImage2Name)
	} else {
		t.Logf("Found image state for %s", testImage2Name)
	}
	imageState3 := imageManager.GetImageStateFromImageName(testImage3Name)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", testImage3Name)
	} else {
		t.Logf("Found image state for %s", testImage3Name)
	}

	imageState1ImageId := imageState1.Image.ImageID
	imageState2ImageId := imageState2.Image.ImageID
	imageState3ImageId := imageState3.Image.ImageID

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
	err = verifyImagesAreRemoved(imageManager, imageState1ImageId, imageState2ImageId)
	if err != nil {
		t.Fatal(err)
	}

	// Verify 3rd LRU image is not removed
	err = verifyImagesAreNotRemoved(imageManager, imageState3ImageId)
	if err != nil {
		t.Fatal(err)
	}

	// Verify top 2 LRU images are removed from docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState1ImageId)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState2ImageId)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}

	// Verify 3rd LRU image has not been removed from Docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState3ImageId)
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
		cleanupImages(imageManager)
	}()

	taskEvents, containerEvents := taskEngine.TaskEvents()
	defer discardEvents(containerEvents)()

	// Create test Task
	taskName := "imgClean"
	testTask := createImageCleanupTestTask(taskName)

	// Start Task
	go taskEngine.AddTask(testTask)

	// Verify that Task is running
	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	imageState1 := imageManager.GetImageStateFromImageName(testImage1Name)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", testImage1Name)
	} else {
		t.Logf("Found image state for %s", testImage1Name)
	}
	imageState2 := imageManager.GetImageStateFromImageName(testImage2Name)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", testImage2Name)
	} else {
		t.Logf("Found image state for %s", testImage2Name)
	}
	imageState3 := imageManager.GetImageStateFromImageName(testImage3Name)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", testImage3Name)
	} else {
		t.Logf("Found image state for %s", testImage3Name)
	}

	imageState1ImageId := imageState1.Image.ImageID
	imageState2ImageId := imageState2.Image.ImageID
	imageState3ImageId := imageState3.Image.ImageID

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
	err = verifyImagesAreRemoved(imageManager, imageState1ImageId, imageState3ImageId)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Image2 is not removed, below threshold for minimumAge
	err = verifyImagesAreNotRemoved(imageManager, imageState2ImageId)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Image1 & Image3 are removed from docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState1ImageId)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState3ImageId)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}

	// Verify Image2 has not been removed from Docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageState2ImageId)
	if err != nil {
		t.Fatalf("Image should not have been removed from Docker")
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

func verifyImagesAreRemoved(imageManager *dockerImageManager, imageIds ...string) error {
	var imagesNotRemovedList *list.List = list.New()
	for _, imageId := range imageIds {
		_, ok := imageManager.getImageState(imageId)
		if ok {
			imagesNotRemovedList.PushFront(imageId)
		}
	}
	if imagesNotRemovedList.Len() > 0 {
		return errors.New(fmt.Sprintf("Image states still exist for: %v", imagesNotRemovedList))
	} else {
		return nil
	}
}

func verifyImagesAreNotRemoved(imageManager *dockerImageManager, imageIds ...string) error {
	var imagesRemovedList *list.List = list.New()
	for _, imageId := range imageIds {
		_, ok := imageManager.getImageState(imageId)
		if !ok {
			imagesRemovedList.PushFront(imageId)
		}
	}
	if imagesRemovedList.Len() > 0 {
		return errors.New(fmt.Sprintf("Could not find images: %v in ImageManager", imagesRemovedList))
	} else {
		return nil
	}
}

func cleanupImages(imageManager *dockerImageManager) {
	imageManager.client.RemoveContainer("test1")
	imageManager.client.RemoveContainer("test2")
	imageManager.client.RemoveContainer("test3")
	imageManager.client.RemoveImage(testImage1Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(testImage2Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(testImage3Name, imageRemovalTimeout)
}
