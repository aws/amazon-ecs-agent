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

const credentialsIDIntegTest = "credsid"

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

// TestImageWithSameNameAndDifferentID tests image can be correctly removed when tasks
// are running with the same image name, but different image id.
func TestImageWithSameNameAndDifferentID(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 15 * time.Minute

	taskEngine, done, _ := setup(cfg, t)
	defer done()

	goDockerClient := taskEngine.(*DockerTaskEngine).client

	imageManager := taskEngine.(*DockerTaskEngine).imageManager.(*dockerImageManager)
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	taskEvents, containerEvents := taskEngine.TaskEvents()
	defer discardEvents(containerEvents)()

	// Pull the images needed for the test
	if _, err := goDockerClient.InspectImage(testImage1Name); err == docker.ErrNoSuchImage {
		metadata := goDockerClient.PullImage(testImage1Name, nil)
		if metadata.Error != nil {
			t.Errorf("Failed to pull image %s", testImage1Name)
		}
	}
	if _, err := goDockerClient.InspectImage(testImage2Name); err == docker.ErrNoSuchImage {
		metadata := goDockerClient.PullImage(testImage2Name, nil)
		if metadata.Error != nil {
			t.Errorf("Failed to pull image %s", testImage2Name)
		}
	}
	if _, err := goDockerClient.InspectImage(testImage3Name); err == docker.ErrNoSuchImage {
		metadata := goDockerClient.PullImage(testImage3Name, nil)
		if metadata.Error != nil {
			t.Errorf("Failed to pull image %s", testImage3Name)
		}
	}

	// The same image name used by all tasks in this test
	identicalImageName := "testimagewithsamenameanddifferentid:latest"
	// Create three tasks which use the image with same name but different ID
	task1 := createTestTask("task1")
	task2 := createTestTask("task2")
	task3 := createTestTask("task3")
	task1.Containers[0].Image = identicalImageName
	task2.Containers[0].Image = identicalImageName
	task3.Containers[0].Image = identicalImageName

	err := renameImage(testImage1Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	if err != nil {
		t.Errorf("Renaming the image failed, err: %v", err)
	}

	// start and wait for task1 to be running
	go taskEngine.AddTask(task1)
	err = verifyTaskIsRunning(taskEvents, task1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify image state is updated correctly
	imageState1 := imageManager.GetImageStateFromImageName(identicalImageName)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", identicalImageName)
	} else {
		t.Logf("Found image state for %s", identicalImageName)
	}
	imageID1 := imageState1.Image.ImageID

	// Using another image but rename to the same name as task1 for task2
	err = renameImage(testImage2Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	if err != nil {
		t.Errorf("Renaming the image failed, err: %v", err)
	}

	// Start and wait for task2 to be running
	go taskEngine.AddTask(task2)
	err = verifyTaskIsRunning(taskEvents, task2)
	if err != nil {
		t.Fatal(err)
	}

	// Verify image state is updated correctly
	imageState2 := imageManager.GetImageStateFromImageName(identicalImageName)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", identicalImageName)
	} else {
		t.Logf("Found image state for %s", identicalImageName)
	}
	imageID2 := imageState2.Image.ImageID
	if imageID2 == imageID1 {
		t.Fatal("The image id in task 2 should be different from image in task 1")
	}

	// Using a different image for task3 and rename it to the same name as task1 and task2
	err = renameImage(testImage3Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	if err != nil {
		t.Errorf("Renaming the image failed, err: %v", err)
	}

	// Start and wiat for task3 to be running
	go taskEngine.AddTask(task3)
	err = verifyTaskIsRunning(taskEvents, task3)
	if err != nil {
		t.Fatal(err)
	}

	// Verify image state is updated correctly
	imageState3 := imageManager.GetImageStateFromImageName(identicalImageName)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", identicalImageName)
	} else {
		t.Logf("Found image state for %s", identicalImageName)
	}
	imageID3 := imageState3.Image.ImageID
	if imageID3 == imageID1 {
		t.Fatal("The image id in task3 should be different from image in task1")
	} else if imageID3 == imageID2 {
		t.Fatal("The image id in task3 should be different from image in task2")
	}

	// Modify image state sothat the image is eligible for deletion
	imageState1.LastUsedAt = imageState1.LastUsedAt.Add(-99995 * time.Hour)
	imageState2.LastUsedAt = imageState2.LastUsedAt.Add(-99994 * time.Hour)
	imageState3.LastUsedAt = imageState3.LastUsedAt.Add(-99993 * time.Hour)

	imageState1.PulledAt = imageState1.PulledAt.Add(-20 * time.Minute)
	imageState2.PulledAt = imageState2.PulledAt.Add(-19 * time.Minute)
	imageState3.PulledAt = imageState3.PulledAt.Add(-18 * time.Minute)

	// Verify Task is stopped
	verifyTaskIsStopped(taskEvents, task1, task2, task3)

	// Allow Task cleanup to occur
	time.Sleep(2 * time.Second)

	err = verifyTaskIsCleanedUp("task1", taskEngine)
	if err != nil {
		t.Fatal(err)
	}
	err = verifyTaskIsCleanedUp("task2", taskEngine)
	if err != nil {
		t.Fatal(err)
	}
	err = verifyTaskIsCleanedUp("task3", taskEngine)
	if err != nil {
		t.Fatal(err)
	}

	imageManager.removeUnusedImages()

	// Verify all the three images are removed from image manager
	err = verifyImagesAreRemoved(imageManager, imageID1, imageID2, imageID3)
	if err != nil {
		t.Fatal(err)
	}

	// Verify images are removed by docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID1)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully, image: %s", imageID1)
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID2)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully, image: %s", imageID2)
	}
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID3)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully, image: %s", imageID3)
	}
}

// TestImageWithSameNameAndDifferentID tests images can be correctly removed if
// tasks are running with the same image id but different image name
func TestImageWithSameIDAndDifferentNames(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 15 * time.Minute

	taskEngine, done, _ := setup(cfg, t)
	defer done()

	goDockerClient := taskEngine.(*DockerTaskEngine).client

	imageManager := taskEngine.(*DockerTaskEngine).imageManager.(*dockerImageManager)
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	taskEvents, containerEvents := taskEngine.TaskEvents()
	defer discardEvents(containerEvents)()

	// Start three tasks which using the image with same ID and different Name
	task1 := createTestTask("task1")
	task2 := createTestTask("task2")
	task3 := createTestTask("task3")
	task1.Containers[0].Image = "testimagewithsameidanddifferentnames-1:latest"
	task2.Containers[0].Image = "testimagewithsameidanddifferentnames-2:latest"
	task3.Containers[0].Image = "testimagewithsameidanddifferentnames-3:latest"

	// Pull the images needed for the test
	if _, err := goDockerClient.InspectImage(testImage1Name); err == docker.ErrNoSuchImage {
		metadata := goDockerClient.PullImage(testImage1Name, nil)
		if metadata.Error != nil {
			t.Errorf("Failed to pull image %s", testImage1Name)
		}
	}

	// Using testImage1Name for all the tasks but with different name
	err := renameImage(testImage1Name, "testimagewithsameidanddifferentnames-1", "latest", goDockerClient)
	if err != nil {
		t.Fatalf("Renaming image failed, err: %v", err)
	}

	// Start and wait for task1 to be running
	go taskEngine.AddTask(task1)
	err = verifyTaskIsRunning(taskEvents, task1)
	if err != nil {
		t.Fatal(err)
	}

	imageState1 := imageManager.GetImageStateFromImageName(task1.Containers[0].Image)
	if imageState1 == nil {
		t.Fatalf("Could not find image state for %s", task1.Containers[0].Image)
	} else {
		t.Logf("Found image state for %s", task1.Containers[0].Image)
	}
	imageID1 := imageState1.Image.ImageID

	// copy the image for task2 to run with same image but different name
	err = goDockerClient.TagImage(task1.Containers[0].Image, docker.TagImageOptions{
		Repo:  "testimagewithsameidanddifferentnames-2",
		Tag:   "latest",
		Force: false,
	}, 1*time.Second)
	if err != nil {
		t.Errorf("Trying to copy image failed, err: %v", err)
	}

	// Start and wait for task2 to be running
	go taskEngine.AddTask(task2)
	err = verifyTaskIsRunning(taskEvents, task2)
	if err != nil {
		t.Fatal(err)
	}

	imageState2 := imageManager.GetImageStateFromImageName(task2.Containers[0].Image)
	if imageState2 == nil {
		t.Fatalf("Could not find image state for %s", task2.Containers[0].Image)
	} else {
		t.Logf("Found image state for %s", task2.Containers[0].Image)
	}
	imageID2 := imageState2.Image.ImageID
	if imageID2 != imageID1 {
		t.Fatal("The image id in task2 should be same as in task1")
	}

	// make task3 use the same image name but different image id
	err = goDockerClient.TagImage(task1.Containers[0].Image, docker.TagImageOptions{
		Repo:  "testimagewithsameidanddifferentnames-3",
		Tag:   "latest",
		Force: false,
	}, 1*time.Second)
	if err != nil {
		t.Errorf("Trying to copy image failed, err: %v", err)
	}

	// Start and wait for task3 to be running
	go taskEngine.AddTask(task3)
	err = verifyTaskIsRunning(taskEvents, task3)
	if err != nil {
		t.Fatal(err)
	}

	imageState3 := imageManager.GetImageStateFromImageName(task3.Containers[0].Image)
	if imageState3 == nil {
		t.Fatalf("Could not find image state for %s", task3.Containers[0].Image)
	} else {
		t.Logf("Found image state for %s", task3.Containers[0].Image)
	}
	imageID3 := imageState3.Image.ImageID
	if imageID3 != imageID1 {
		t.Fatal("The image id in task3 should be the same as in task1")
	}

	// Modify the image state so that the image is eligible for deletion
	// all the three tasks has the same imagestate
	imageState1.LastUsedAt = imageState1.LastUsedAt.Add(-99995 * time.Hour)
	imageState1.PulledAt = imageState1.PulledAt.Add(-20 * time.Minute)

	// Verify Task is stopped
	verifyTaskIsStopped(taskEvents, task1, task2, task3)

	// Allow Task cleanup to occur
	time.Sleep(2 * time.Second)

	err = verifyTaskIsCleanedUp("task1", taskEngine)
	if err != nil {
		t.Fatal(err)
	}
	err = verifyTaskIsCleanedUp("task2", taskEngine)
	if err != nil {
		t.Fatal(err)
	}
	err = verifyTaskIsCleanedUp("task3", taskEngine)
	if err != nil {
		t.Fatal(err)
	}

	imageManager.removeUnusedImages()

	// Verify all the images are removed from image manager
	err = verifyImagesAreRemoved(imageManager, imageID1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify images are removed by docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID1)
	if err != docker.ErrNoSuchImage {
		t.Fatalf("Image was not removed successfully")
	}
}

// renameImage retag the image and delete the original tag
func renameImage(original, repo, tag string, client DockerClient) error {
	err := client.TagImage(original, docker.TagImageOptions{
		Repo:  repo,
		Tag:   tag,
		Force: false,
	}, 1*time.Second)
	if err != nil {
		return fmt.Errorf("Trying to tag image failed, err: %v", err)
	}

	// delete the original tag
	err = client.RemoveImage(original, imageRemovalTimeout)
	if err != nil {
		return fmt.Errorf("Failed to remove the original tag of the image: %s", original)
	}

	return nil
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

func cleanupImages(imageManager *dockerImageManager) {
	imageManager.client.RemoveContainer("test1", removeContainerTimeout)
	imageManager.client.RemoveContainer("test2", removeContainerTimeout)
	imageManager.client.RemoveContainer("test3", removeContainerTimeout)
	imageManager.client.RemoveImage(testImage1Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(testImage2Name, imageRemovalTimeout)
	imageManager.client.RemoveImage(testImage3Name, imageRemovalTimeout)
}
