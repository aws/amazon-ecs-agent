// +build !windows,integration
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
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	test1Image1Name = "127.0.0.1:51670/amazon/image-cleanup-test-image1:latest"
	test1Image2Name = "127.0.0.1:51670/amazon/image-cleanup-test-image2:latest"
	test1Image3Name = "127.0.0.1:51670/amazon/image-cleanup-test-image3:latest"

	test2Image1Name = "127.0.0.1:51670/amazon/image-cleanup-test-image1:latest"
	test2Image2Name = "127.0.0.1:51670/amazon/image-cleanup-test-image2:latest"
	test2Image3Name = "127.0.0.1:51670/amazon/image-cleanup-test-image3:latest"

	test3Image1Name = "127.0.0.1:51670/amazon/image-cleanup-test-image1:latest"
	test3Image2Name = "127.0.0.1:51670/amazon/image-cleanup-test-image2:latest"
	test3Image3Name = "127.0.0.1:51670/amazon/image-cleanup-test-image3:latest"

	test4Image1Name = "127.0.0.1:51670/amazon/image-cleanup-test-image1:latest"
)

// TestImageWithSameNameAndDifferentID tests image can be correctly removed when tasks
// are running with the same image name, but different image id.
func TestImageWithSameNameAndDifferentID(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 15 * time.Minute

	taskEngine, done, _ := setup(cfg, t)
	defer done()

	dockerClient := taskEngine.(*DockerTaskEngine).client

	// DockerClient doesn't implement TagImage, create a go docker client
	goDockerClient, err := docker.NewClientFromEnv()
	require.NoError(t, err, "Creating go docker client failed")

	imageManager := taskEngine.(*DockerTaskEngine).imageManager.(*dockerImageManager)
	imageManager.SetSaver(statemanager.NewNoopStateManager())

	taskEvents, containerEvents := taskEngine.TaskEvents()
	defer discardEvents(containerEvents)()

	// Pull the images needed for the test
	if _, err = dockerClient.InspectImage(test3Image1Name); err == docker.ErrNoSuchImage {
		metadata := dockerClient.PullImage(test3Image1Name, nil)
		assert.NoError(t, metadata.Error, "Failed to pull image %s", test3Image1Name)
	}
	if _, err = dockerClient.InspectImage(test3Image2Name); err == docker.ErrNoSuchImage {
		metadata := dockerClient.PullImage(test3Image2Name, nil)
		assert.NoError(t, metadata.Error, "Failed to pull image %s", test3Image2Name)
	}
	if _, err = dockerClient.InspectImage(test3Image3Name); err == docker.ErrNoSuchImage {
		metadata := dockerClient.PullImage(test3Image3Name, nil)
		assert.NoError(t, metadata.Error, "Failed to pull image %s", test3Image3Name)
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

	err = renameImage(test3Image1Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	assert.NoError(t, err, "Renaming the image failed")

	// start and wait for task1 to be running
	go taskEngine.AddTask(task1)
	err = verifyTaskIsRunning(taskEvents, task1)
	require.NoError(t, err, "task1")

	// Verify image state is updated correctly
	imageState1 := imageManager.GetImageStateFromImageName(identicalImageName)
	require.NotNil(t, imageState1, "Could not find image state for %s", identicalImageName)
	t.Logf("Found image state for %s", identicalImageName)
	imageID1 := imageState1.Image.ImageID

	// Using another image but rename to the same name as task1 for task2
	err = renameImage(test3Image2Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	require.NoError(t, err, "Renaming the image failed")

	// Start and wait for task2 to be running
	go taskEngine.AddTask(task2)
	err = verifyTaskIsRunning(taskEvents, task2)
	require.NoError(t, err, "task2")

	// Verify image state is updated correctly
	imageState2 := imageManager.GetImageStateFromImageName(identicalImageName)
	require.NotNil(t, imageState2, "Could not find image state for %s", identicalImageName)
	t.Logf("Found image state for %s", identicalImageName)
	imageID2 := imageState2.Image.ImageID
	require.NotEqual(t, imageID2, imageID1, "The image id in task 2 should be different from image in task 1")

	// Using a different image for task3 and rename it to the same name as task1 and task2
	err = renameImage(test3Image3Name, "testimagewithsamenameanddifferentid", "latest", goDockerClient)
	require.NoError(t, err, "Renaming the image failed")

	// Start and wiat for task3 to be running
	go taskEngine.AddTask(task3)
	err = verifyTaskIsRunning(taskEvents, task3)
	require.NoError(t, err, "task3")

	// Verify image state is updated correctly
	imageState3 := imageManager.GetImageStateFromImageName(identicalImageName)
	require.NotNil(t, imageState3, "Could not find image state for %s", identicalImageName)
	t.Logf("Found image state for %s", identicalImageName)
	imageID3 := imageState3.Image.ImageID
	require.NotEqual(t, imageID3, imageID1, "The image id in task3 should be different from image in task1")
	require.NotEqual(t, imageID3, imageID2, "The image id in task3 should be different from image in task2")

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
	assert.NoError(t, err, "task1")
	err = verifyTaskIsCleanedUp("task2", taskEngine)
	assert.NoError(t, err, "task2")
	err = verifyTaskIsCleanedUp("task3", taskEngine)
	assert.NoError(t, err, "task3")

	imageManager.removeUnusedImages()

	// Verify all the three images are removed from image manager
	err = verifyImagesAreRemoved(imageManager, imageID1, imageID2, imageID3)
	require.NoError(t, err)

	// Verify images are removed by docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID1)
	assert.Equal(t, docker.ErrNoSuchImage, err, "Image was not removed successfully, image: %s", imageID1)
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID2)
	assert.Equal(t, docker.ErrNoSuchImage, err, "Image was not removed successfully, image: %s", imageID2)
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID3)
	assert.Equal(t, docker.ErrNoSuchImage, err, "Image was not removed successfully, image: %s", imageID3)
}

// TestImageWithSameIDAndDifferentNames tests images can be correctly removed if
// tasks are running with the same image id but different image name
func TestImageWithSameIDAndDifferentNames(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second

	// Set low values so this test can complete in a sane amout of time
	cfg.MinimumImageDeletionAge = 15 * time.Minute

	taskEngine, done, _ := setup(cfg, t)
	defer done()

	dockerClient := taskEngine.(*DockerTaskEngine).client

	// DockerClient doesn't implement TagImage, so create a go docker client
	goDockerClient, err := docker.NewClientFromEnv()
	require.NoError(t, err, "Creating docker client failed")

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
	if _, err = dockerClient.InspectImage(test4Image1Name); err == docker.ErrNoSuchImage {
		metadata := dockerClient.PullImage(test4Image1Name, nil)
		assert.NoError(t, metadata.Error, "Failed to pull image %s", test4Image1Name)
	}

	// Using testImage1Name for all the tasks but with different name
	err = renameImage(test4Image1Name, "testimagewithsameidanddifferentnames-1", "latest", goDockerClient)
	require.NoError(t, err, "Renaming image failed")

	// Start and wait for task1 to be running
	go taskEngine.AddTask(task1)
	err = verifyTaskIsRunning(taskEvents, task1)
	require.NoError(t, err)

	imageState1 := imageManager.GetImageStateFromImageName(task1.Containers[0].Image)
	require.NotNil(t, imageState1, "Could not find image state for %s", task1.Containers[0].Image)
	t.Logf("Found image state for %s", task1.Containers[0].Image)
	imageID1 := imageState1.Image.ImageID

	// copy the image for task2 to run with same image but different name
	err = goDockerClient.TagImage(task1.Containers[0].Image, docker.TagImageOptions{
		Repo:  "testimagewithsameidanddifferentnames-2",
		Tag:   "latest",
		Force: false,
	})
	require.NoError(t, err, "Trying to copy image failed")

	// Start and wait for task2 to be running
	go taskEngine.AddTask(task2)
	err = verifyTaskIsRunning(taskEvents, task2)
	require.NoError(t, err)

	imageState2 := imageManager.GetImageStateFromImageName(task2.Containers[0].Image)
	require.NotNil(t, imageState2, "Could not find image state for %s", task2.Containers[0].Image)
	t.Logf("Found image state for %s", task2.Containers[0].Image)
	imageID2 := imageState2.Image.ImageID
	require.Equal(t, imageID2, imageID1, "The image id in task2 should be same as in task1")

	// make task3 use the same image name but different image id
	err = goDockerClient.TagImage(task1.Containers[0].Image, docker.TagImageOptions{
		Repo:  "testimagewithsameidanddifferentnames-3",
		Tag:   "latest",
		Force: false,
	})
	require.NoError(t, err, "Trying to copy image failed")

	// Start and wait for task3 to be running
	go taskEngine.AddTask(task3)
	err = verifyTaskIsRunning(taskEvents, task3)
	assert.NoError(t, err)

	imageState3 := imageManager.GetImageStateFromImageName(task3.Containers[0].Image)
	require.NotNil(t, imageState3, "Could not find image state for %s", task3.Containers[0].Image)
	t.Logf("Found image state for %s", task3.Containers[0].Image)
	imageID3 := imageState3.Image.ImageID
	require.Equal(t, imageID3, imageID1, "The image id in task3 should be the same as in task1")

	// Modify the image state so that the image is eligible for deletion
	// all the three tasks has the same imagestate
	imageState1.LastUsedAt = imageState1.LastUsedAt.Add(-99995 * time.Hour)
	imageState1.PulledAt = imageState1.PulledAt.Add(-20 * time.Minute)

	// Verify Task is stopped
	verifyTaskIsStopped(taskEvents, task1, task2, task3)

	// Allow Task cleanup to occur
	time.Sleep(2 * time.Second)

	err = verifyTaskIsCleanedUp("task1", taskEngine)
	assert.NoError(t, err, "task1")
	err = verifyTaskIsCleanedUp("task2", taskEngine)
	assert.NoError(t, err, "task2")
	err = verifyTaskIsCleanedUp("task3", taskEngine)
	assert.NoError(t, err, "task3")

	imageManager.removeUnusedImages()

	// Verify all the images are removed from image manager
	err = verifyImagesAreRemoved(imageManager, imageID1)
	assert.NoError(t, err, "imageID1")

	// Verify images are removed by docker
	_, err = taskEngine.(*DockerTaskEngine).client.InspectImage(imageID1)
	assert.Equal(t, docker.ErrNoSuchImage, err, "Image was not removed successfully")
}

// renameImage retag the image and delete the original tag
func renameImage(original, repo, tag string, client *docker.Client) error {
	err := client.TagImage(original, docker.TagImageOptions{
		Repo:  repo,
		Tag:   tag,
		Force: false,
	})
	if err != nil {
		return fmt.Errorf("Trying to tag image failed, err: %v", err)
	}

	// delete the original tag
	err = client.RemoveImage(original)
	if err != nil {
		return fmt.Errorf("Failed to remove the original tag of the image: %s", original)
	}

	return nil
}
