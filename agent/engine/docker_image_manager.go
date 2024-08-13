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
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	"github.com/docker/docker/api/types"
)

const (
	imageNotFoundForDeletionError = "no such image"
)

// ImageManager is responsible for saving the Image states,
// adding and removing container references to ImageStates
type ImageManager interface {
	RecordContainerReference(container *apicontainer.Container) error
	RemoveContainerReferenceFromImageState(container *apicontainer.Container) error
	AddAllImageStates(imageStates []*image.ImageState)
	GetImageStateFromImageName(containerImageName string) (*image.ImageState, bool)
	StartImageCleanupProcess(ctx context.Context)
	SetDataClient(dataClient data.Client)
	AddImageToCleanUpExclusionList(image string)
}

// dockerImageManager accounts all the images and their states in the instance.
// It also has the cleanup policy configuration.
type dockerImageManager struct {
	imageStates                        []*image.ImageState
	client                             dockerapi.DockerClient
	dataClient                         data.Client
	updateLock                         sync.RWMutex
	imageCleanupTicker                 *time.Ticker
	state                              dockerstate.TaskEngineState
	imageStatesConsideredForDeletion   map[string]*image.ImageState
	minimumAgeBeforeDeletion           time.Duration
	numImagesToDelete                  int
	imageCleanupTimeInterval           time.Duration
	imagePullBehavior                  config.ImagePullBehaviorType
	imageCleanupExclusionList          []string
	deleteNonECSImagesEnabled          config.BooleanDefaultFalse
	nonECSContainerCleanupWaitDuration time.Duration
	numNonECSContainersToDelete        int
	nonECSMinimumAgeBeforeDeletion     time.Duration
}

// ImageStatesForDeletion is used for implementing the sort interface
type ImageStatesForDeletion []*image.ImageState

// NewImageManager returns a new ImageManager
func NewImageManager(cfg *config.Config, client dockerapi.DockerClient, state dockerstate.TaskEngineState) ImageManager {
	return &dockerImageManager{
		client:                             client,
		state:                              state,
		minimumAgeBeforeDeletion:           cfg.MinimumImageDeletionAge,
		numImagesToDelete:                  cfg.NumImagesToDeletePerCycle,
		imageCleanupTimeInterval:           cfg.ImageCleanupInterval,
		imagePullBehavior:                  cfg.ImagePullBehavior,
		imageCleanupExclusionList:          buildImageCleanupExclusionList(cfg),
		deleteNonECSImagesEnabled:          cfg.DeleteNonECSImagesEnabled,
		nonECSContainerCleanupWaitDuration: cfg.TaskCleanupWaitDuration,
		numNonECSContainersToDelete:        cfg.NumNonECSContainersToDeletePerCycle,
		nonECSMinimumAgeBeforeDeletion:     cfg.NonECSMinimumImageDeletionAge,
	}
}

// SetDataClient sets the saver that is used by the ImageManager.
func (imageManager *dockerImageManager) SetDataClient(dataClient data.Client) {
	imageManager.dataClient = dataClient
}

func buildImageCleanupExclusionList(cfg *config.Config) []string {
	// append known cached internal images to imageCleanupExclusionList
	excludedImages := append(cfg.ImageCleanupExclusionList,
		cfg.PauseContainerImageName+":"+cfg.PauseContainerTag,
		config.DefaultPauseContainerImageName+":"+config.DefaultPauseContainerTag,
		config.CachedImageNameAgentContainer,
	)
	for _, image := range excludedImages {
		logger.Info("Image excluded from cleanup", logger.Fields{
			field.Image: image,
		})
	}
	return excludedImages
}

func (imageManager *dockerImageManager) AddImageToCleanUpExclusionList(image string) {
	imageManager.updateLock.Lock()
	defer imageManager.updateLock.Unlock()
	imageManager.imageCleanupExclusionList = append(imageManager.imageCleanupExclusionList, image)
	logger.Info("Image excluded from cleanup", logger.Fields{
		field.Image: image,
	})
}

func (imageManager *dockerImageManager) AddAllImageStates(imageStates []*image.ImageState) {
	imageManager.updateLock.Lock()
	defer imageManager.updateLock.Unlock()
	for _, imageState := range imageStates {
		imageManager.addImageState(imageState)
	}
}

func (imageManager *dockerImageManager) GetImageStatesCount() int {
	imageManager.updateLock.RLock()
	defer imageManager.updateLock.RUnlock()
	return len(imageManager.imageStates)
}

// RecordContainerReference adds container reference to the corresponding imageState object
func (imageManager *dockerImageManager) RecordContainerReference(container *apicontainer.Container) error {
	// On agent restart, container ID was retrieved from agent state file
	// TODO add setter and getter for modifying this
	if container.ImageID != "" {
		if !imageManager.addContainerReferenceToExistingImageState(container) {
			return fmt.Errorf("Failed to add container to existing image state")
		}
		return nil
	}

	if container.Image == "" {
		return fmt.Errorf("Invalid container reference: Empty image name")
	}

	// Inspect image for obtaining Container's Image ID
	imageInspected, err := imageManager.client.InspectImage(container.Image)
	if err != nil {
		fields := container.Fields()
		fields[field.Error] = err
		logger.Error("Error inspecting image", fields)
		return err
	}
	container.ImageID = imageInspected.ID
	// For older Docker versions imageDigest is not populated during transition to
	// MANIFEST_PULLED state. Populate it here if that's the case.
	if container.GetImageDigest() == "" {
		imageDigest := imageManager.fetchRepoDigest(imageInspected, container)
		container.SetImageDigest(imageDigest)
	}
	added := imageManager.addContainerReferenceToExistingImageState(container)
	if !added {
		imageManager.addContainerReferenceToNewImageState(container, imageInspected.Size)
	}
	return nil
}

// The helper function to fetch the RepoImageDigest when inspect the image
func (imageManager *dockerImageManager) fetchRepoDigest(imageInspected *types.ImageInspect, container *apicontainer.Container) string {
	imageRepoDigests := imageInspected.RepoDigests
	resultRepoDigest := ""
	imagePrefix := strings.Split(container.Image, ":")[0]
	for _, imageRepoDigest := range imageRepoDigests {
		if strings.HasPrefix(imageRepoDigest, imagePrefix) {
			repoDigestSplitList := strings.Split(imageRepoDigest, "@")
			if len(repoDigestSplitList) > 1 {
				resultRepoDigest = repoDigestSplitList[1]
				return resultRepoDigest
			} else {
				logger.Warn(fmt.Sprintf("ImageRepoDigest doesn't have the right format: %v", imageRepoDigest), container.Fields())
				return ""
			}
		}
	}
	return resultRepoDigest
}

func (imageManager *dockerImageManager) addContainerReferenceToExistingImageState(container *apicontainer.Container) bool {
	// this lock is used for reading the image states in the image manager
	imageManager.updateLock.RLock()
	defer imageManager.updateLock.RUnlock()
	imageManager.removeExistingImageNameOfDifferentID(container.Image, container.ImageID)
	imageState, ok := imageManager.getImageState(container.ImageID)
	if ok {
		imageState.UpdateImageState(container)
		imageManager.saveImageStateData(imageState)
	}
	return ok
}

func (imageManager *dockerImageManager) addContainerReferenceToNewImageState(container *apicontainer.Container, imageSize int64) {
	// this lock is used while creating and adding new image state to image manager
	imageManager.updateLock.Lock()
	defer imageManager.updateLock.Unlock()
	imageManager.removeExistingImageNameOfDifferentID(container.Image, container.ImageID)
	// check to see if a different thread added image state for same image ID
	imageState, ok := imageManager.getImageState(container.ImageID)
	if ok {
		imageState.UpdateImageState(container)
		imageManager.saveImageStateData(imageState)
	} else {
		sourceImage := &image.Image{
			ImageID: container.ImageID,
			Size:    imageSize,
		}
		sourceImageState := &image.ImageState{
			Image:      sourceImage,
			PulledAt:   time.Now(),
			LastUsedAt: time.Now(),
		}
		sourceImageState.UpdateImageState(container)
		imageManager.addImageState(sourceImageState)
	}
}

// RemoveContainerReferenceFromImageState removes container reference from the corresponding imageState object
func (imageManager *dockerImageManager) RemoveContainerReferenceFromImageState(container *apicontainer.Container) error {
	// this lock is for reading image states and finding the one that the container belongs to
	imageManager.updateLock.RLock()
	defer imageManager.updateLock.RUnlock()
	if container.ImageID == "" {
		return fmt.Errorf("Invalid container reference: Empty image id")
	}

	// Find image state that this container is part of, and remove the reference
	imageState, ok := imageManager.getImageState(container.ImageID)
	if !ok {
		return fmt.Errorf("Cannot find image state for the container to be removed")
	}
	// Found matching ImageState
	err := imageState.RemoveContainerReference(container)
	if err != nil {
		return err
	}
	imageManager.saveImageStateData(imageState)
	return nil
}

func (imageManager *dockerImageManager) addImageState(imageState *image.ImageState) {
	imageManager.imageStates = append(imageManager.imageStates, imageState)
	imageManager.saveImageStateData(imageState)

}

// getAllImageStates returns the list of imageStates in the instance
func (imageManager *dockerImageManager) getAllImageStates() []*image.ImageState {
	return imageManager.imageStates
}

// getImageState returns the ImageState object that the container is referenced at
func (imageManager *dockerImageManager) getImageState(containerImageID string) (*image.ImageState, bool) {
	for _, imageState := range imageManager.getAllImageStates() {
		if imageState.Image.ImageID == containerImageID {
			return imageState, true
		}
	}
	return nil, false
}

// removeImageState removes the imageState from the list of imageState objects in ImageManager
func (imageManager *dockerImageManager) removeImageState(imageStateToBeRemoved *image.ImageState) {
	for i, imageState := range imageManager.imageStates {
		if imageState.Image.ImageID == imageStateToBeRemoved.Image.ImageID {
			// Image State found; hence remove it
			logger.Debug("Image Manager: removing image state", imageState.Fields())
			imageManager.imageStates = append(imageManager.imageStates[:i], imageManager.imageStates[i+1:]...)
			imageManager.removeImageStateData(imageState.Image.ImageID)
			return
		}
	}
}

func (imageManager *dockerImageManager) getCandidateImagesForDeletion() []*image.ImageState {
	if len(imageManager.imageStatesConsideredForDeletion) < 1 {
		logger.Debug("Image Manager: Empty state!")
		// no image states present in image manager
		return nil
	}
	var imagesForDeletion []*image.ImageState
	for _, imageState := range imageManager.imageStatesConsideredForDeletion {
		if imageManager.isImageOldEnough(imageState) && imageState.HasNoAssociatedContainers() {
			logger.Debug("Candidate image for deletion", imageState.Fields())
			imagesForDeletion = append(imagesForDeletion, imageState)
		}
	}
	return imagesForDeletion
}

func (imageManager *dockerImageManager) isImageOldEnough(imageState *image.ImageState) bool {
	ageOfImage := time.Since(imageState.PulledAt)
	return ageOfImage > imageManager.minimumAgeBeforeDeletion
}

// TODO: change image createdTime to image lastUsedTime when docker support it in the future
func (imageManager *dockerImageManager) nonECSImageOldEnough(NonECSImage ImageWithSizeID) bool {
	ageOfImage := time.Since(NonECSImage.createdTime)
	return ageOfImage > imageManager.nonECSMinimumAgeBeforeDeletion
}

// Implementing sort interface based on last used times of the images
func (imageStates ImageStatesForDeletion) Len() int {
	return len(imageStates)
}

func (imageStates ImageStatesForDeletion) Less(i, j int) bool {
	return imageStates[i].LastUsedAt.Before(imageStates[j].LastUsedAt)
}

func (imageStates ImageStatesForDeletion) Swap(i, j int) {
	imageStates[i], imageStates[j] = imageStates[j], imageStates[i]
}

func (imageManager *dockerImageManager) getLeastRecentlyUsedImage(imagesForDeletion []*image.ImageState) *image.ImageState {
	var candidateImages ImageStatesForDeletion
	for _, imageState := range imagesForDeletion {
		candidateImages = append(candidateImages, imageState)
	}
	// sort images in the order of last used times
	sort.Sort(candidateImages)
	// return only the top LRU image for deletion
	return candidateImages[0]
}

func (imageManager *dockerImageManager) removeExistingImageNameOfDifferentID(containerImageName string, inspectedImageID string) {
	for _, imageState := range imageManager.getAllImageStates() {
		// image with same name pulled in the instance. Untag the already existing image name
		if imageState.Image.ImageID != inspectedImageID {
			if imageNameRemoved := imageState.RemoveImageName(containerImageName); imageNameRemoved {
				imageManager.saveImageStateData(imageState)
			}
		}
	}
}

func (imageManager *dockerImageManager) StartImageCleanupProcess(ctx context.Context) {
	// If the image pull behavior is prefer cached, don't clean up the image,
	// because the cached image is needed.
	if imageManager.imagePullBehavior == config.ImagePullPreferCachedBehavior {
		logger.Info("Pull behavior is set to always use cache. Disabling cleanup")
		return
	}
	// passing the cleanup interval as argument which would help during testing
	imageManager.performPeriodicImageCleanup(ctx, imageManager.imageCleanupTimeInterval)
}

func (imageManager *dockerImageManager) performPeriodicImageCleanup(ctx context.Context, imageCleanupInterval time.Duration) {
	imageManager.imageCleanupTicker = time.NewTicker(imageCleanupInterval)
	for {
		select {
		case <-imageManager.imageCleanupTicker.C:
			go imageManager.removeUnusedImages(ctx)
		case <-ctx.Done():
			imageManager.imageCleanupTicker.Stop()
			return
		}
	}
}

func (imageManager *dockerImageManager) removeUnusedImages(ctx context.Context) {
	logger.Debug("Attempting to obtain ImagePullDeleteLock for removing images")
	ImagePullDeleteLock.Lock()
	logger.Debug("Obtained ImagePullDeleteLock for removing images")
	defer logger.Debug("Released ImagePullDeleteLock after removing images")
	defer ImagePullDeleteLock.Unlock()

	imageManager.updateLock.Lock()
	defer imageManager.updateLock.Unlock()

	var numECSImagesDeleted int
	imageManager.imageStatesConsideredForDeletion = imageManager.imagesConsiderForDeletion(imageManager.getAllImageStates())

	for i := 0; i < imageManager.numImagesToDelete; i++ {
		err := imageManager.removeLeastRecentlyUsedImage(ctx)
		numECSImagesDeleted = i
		if err != nil {
			logger.Info("End of eligible images for deletion", logger.Fields{
				"managedImagesRemaining": len(imageManager.getAllImageStates()),
			})
			break
		}
	}
	if imageManager.deleteNonECSImagesEnabled.Enabled() {
		// remove nonecs containers
		imageManager.removeNonECSContainers(ctx)
		// remove nonecs images
		var nonECSImagesNumToDelete = imageManager.numImagesToDelete - numECSImagesDeleted
		imageManager.removeNonECSImages(ctx, nonECSImagesNumToDelete)
	}
}

func (imageManager *dockerImageManager) removeNonECSContainers(ctx context.Context) {
	nonECSContainersIDs, err := imageManager.getNonECSContainerIDs(ctx)
	if err != nil {
		logger.Error(fmt.Sprintf("Error getting non-ECS container IDs: %v", err))
	}
	var nonECSContainerRemoveAvailableIDs []string
	for _, id := range nonECSContainersIDs {
		response, icErr := imageManager.client.InspectContainer(ctx, id, dockerclient.InspectContainerTimeout)
		if icErr != nil {
			logger.Error(fmt.Sprintf("Error inspecting non-ECS container id: %s - %v", id, icErr))
			continue
		}

		logger.Debug(fmt.Sprintf("Inspecting Non-ECS Container ID [%s] for removal, Finished [%s] Status [%s]", id, response.State.FinishedAt, response.State.Status))
		finishedTime, err := time.Parse(time.RFC3339Nano, response.State.FinishedAt)
		if err != nil {
			logger.Error(fmt.Sprintf("Error parsing time string for container. id: %s, time: %s err: %s", id, response.State.FinishedAt, err))
			continue
		}

		if (response.State.Status == "exited" ||
			response.State.Status == "dead" ||
			response.State.Status == "created") &&
			time.Since(finishedTime) > imageManager.nonECSContainerCleanupWaitDuration {
			nonECSContainerRemoveAvailableIDs = append(nonECSContainerRemoveAvailableIDs, id)
		}
	}
	var numNonECSContainerDeleted = 0
	for _, id := range nonECSContainerRemoveAvailableIDs {
		if numNonECSContainerDeleted == imageManager.numNonECSContainersToDelete {
			break
		}
		logger.Debug("Removing non-ECS container", logger.Fields{
			"runtimeID": id,
		})
		err := imageManager.client.RemoveContainer(ctx, id, dockerclient.RemoveContainerTimeout)
		if err == nil {
			logger.Info("Removed non-ECS container", logger.Fields{
				"runtimeID": id,
			})
			numNonECSContainerDeleted++
		} else {
			logger.Error(fmt.Sprintf("Error Removing Container ID %s - %s", id, err))
			continue
		}
	}
}

func (imageManager *dockerImageManager) getNonECSContainerIDs(ctx context.Context) ([]string, error) {
	var allContainersIDs []string
	listContainersResponse := imageManager.client.ListContainers(ctx, true, dockerclient.ListContainersTimeout)
	if listContainersResponse.Error != nil {
		return nil, fmt.Errorf("Error listing containers: %v", listContainersResponse.Error)
	}
	allContainersIDs = append(allContainersIDs, listContainersResponse.DockerIDs...)
	ECSContainersIDs := imageManager.state.GetAllContainerIDs()
	nonECSContainersIDs := exclude(allContainersIDs, ECSContainersIDs)
	return nonECSContainersIDs, nil
}

type ImageWithSizeID struct {
	RepoTags    []string
	ImageID     string
	Size        int64
	createdTime time.Time
}

func (imageManager *dockerImageManager) removeNonECSImages(ctx context.Context, nonECSImagesNumToDelete int) {
	if nonECSImagesNumToDelete == 0 {
		return
	}
	nonECSImages := imageManager.getNonECSImages(ctx)

	// we want to sort images with size ascending
	sort.Slice(nonECSImages, func(i, j int) bool {
		return nonECSImages[i].Size < nonECSImages[j].Size
	})

	// we will remove the remaining nonECSImages in each performPeriodicImageCleanup call()
	var numImagesAlreadyDeleted = 0
	for _, image := range nonECSImages {
		if numImagesAlreadyDeleted >= nonECSImagesNumToDelete {
			break
		}
		// use current time - image creation time to determine if image is old enough to be deleted.
		if !imageManager.nonECSImageOldEnough(image) {
			continue
		}
		fields := logger.Fields{
			field.ImageID:        image.ImageID,
			field.ImageSizeBytes: image.Size,
			"repoTags":           image.RepoTags,
		}
		if len(image.RepoTags) > 1 {
			logger.Debug("Non-ECS image has more than one tag", fields)
			for _, tag := range image.RepoTags {
				err := imageManager.client.RemoveImage(ctx, tag, dockerclient.RemoveImageTimeout)
				if err != nil {
					logger.Error("Error removing non-ECS RepoTag", fields, logger.Fields{
						field.Error: err,
						"imageTag":  tag,
					})
				} else {
					logger.Info("Non-ECS image tag removed", fields, logger.Fields{"imageTag": tag})
					numImagesAlreadyDeleted++
				}
			}
		} else {
			logger.Debug("Removing non-ECS image", fields)
			err := imageManager.client.RemoveImage(ctx, image.ImageID, dockerclient.RemoveImageTimeout)
			if err != nil {
				logger.Error("Error removing non-ECS image", fields, logger.Fields{field.Error: err})
			} else {
				logger.Info("Non-ECS image removed", fields)
				numImagesAlreadyDeleted++
			}
		}
	}
}

// getNonECSImages returns type ImageWithSizeID with all fields populated.
func (imageManager *dockerImageManager) getNonECSImages(ctx context.Context) []ImageWithSizeID {
	r := imageManager.client.ListImages(ctx, dockerclient.ListImagesTimeout)
	var allImages []ImageWithSizeID
	// inspect all images
	for _, imageID := range r.ImageIDs {
		resp, err := imageManager.client.InspectImage(imageID)
		if err != nil {
			logger.Error("Error inspecting non-ECS image", logger.Fields{field.ImageID: imageID, field.Error: err})
			continue
		}
		createTime := time.Time{}
		createTime, err = time.Parse(time.RFC3339, resp.Created)
		if err != nil {
			logger.Warn("Error parsing the inspected non-ECS image created time.",
				logger.Fields{field.ImageID: imageID, field.Error: err})
		}
		allImages = append(allImages,
			ImageWithSizeID{
				ImageID:     imageID,
				Size:        resp.Size,
				RepoTags:    resp.RepoTags,
				createdTime: createTime,
			})
	}

	// get all 'ecs' image IDs
	var ecsImageIDs []string
	for _, imageState := range imageManager.getAllImageStates() {
		ecsImageIDs = append(ecsImageIDs, imageState.Image.ImageID)
	}

	// exclude 'ecs' image IDs and image IDs with an explicitly excluded tag
	var nonECSImages []ImageWithSizeID
	for _, image := range allImages {
		// check image ID is not excluded
		if isInExclusionList(image.ImageID, ecsImageIDs) {
			continue
		}
		// check image TAG(s) is not excluded
		if !anyIsInExclusionList(image.RepoTags, imageManager.imageCleanupExclusionList) {
			nonECSImages = append(nonECSImages, image)
		}
	}
	return nonECSImages
}

func isInExclusionList(imageName string, imageExclusionList []string) bool {
	for _, exclusionName := range imageExclusionList {
		if imageName == exclusionName {
			return true
		}
	}
	return false
}

// anyIsInExclusionList returns true if any name is in the exclusion list.
func anyIsInExclusionList(imageNames []string, nameExclusionList []string) bool {
	for _, name := range imageNames {
		if isInExclusionList(name, nameExclusionList) {
			return true
		}
	}
	return false
}

func exclude(allList []string, exclusionList []string) []string {
	var ret []string
	var allMap = make(map[string]bool)
	for _, a := range allList {
		allMap[a] = true
	}
	for _, b := range exclusionList {
		allMap[b] = false
	}
	for k := range allMap {
		if allMap[k] {
			ret = append(ret, k)
		}
	}
	return ret
}

func (imageManager *dockerImageManager) imagesConsiderForDeletion(allImageStates []*image.ImageState) map[string]*image.ImageState {
	var imagesConsiderForDeletionMap = make(map[string]*image.ImageState)
	logger.Debug("Begin building map of eligible unused images for deletion")
	for _, imageState := range allImageStates {
		if imageManager.isExcludedFromCleanup(imageState) {
			//imageState that we want to keep
			logger.Debug("Image excluded from deletion", imageState.Fields())
		} else {
			logger.Debug("Image going to be considered for deletion", imageState.Fields())
			imagesConsiderForDeletionMap[imageState.Image.ImageID] = imageState
		}
	}
	return imagesConsiderForDeletionMap
}

func (imageManager *dockerImageManager) isExcludedFromCleanup(imageState *image.ImageState) bool {
	for _, ecsName := range imageState.Image.Names {
		for _, exclusionName := range imageManager.imageCleanupExclusionList {
			if ecsName == exclusionName {
				return true
			}
		}
	}
	return false
}

func (imageManager *dockerImageManager) removeLeastRecentlyUsedImage(ctx context.Context) error {
	leastRecentlyUsedImage := imageManager.getUnusedImageForDeletion()
	if leastRecentlyUsedImage == nil {
		return fmt.Errorf("No more eligible images for deletion")
	}
	logger.Info("Image ready for deletion", leastRecentlyUsedImage.Fields())
	imageManager.removeImage(ctx, leastRecentlyUsedImage)
	return nil
}

func (imageManager *dockerImageManager) getUnusedImageForDeletion() *image.ImageState {
	candidateImageStatesForDeletion := imageManager.getCandidateImagesForDeletion()
	if len(candidateImageStatesForDeletion) < 1 {
		logger.Debug("No eligible images for deletion for this cleanup cycle")
		return nil
	}
	logger.Debug(fmt.Sprintf("Found %d eligible images for deletion", len(candidateImageStatesForDeletion)))
	return imageManager.getLeastRecentlyUsedImage(candidateImageStatesForDeletion)
}

func (imageManager *dockerImageManager) removeImage(ctx context.Context, leastRecentlyUsedImage *image.ImageState) {
	// Handling deleting while traversing a slice
	imageNames := make([]string, len(leastRecentlyUsedImage.Image.Names))
	copy(imageNames, leastRecentlyUsedImage.Image.Names)
	if len(imageNames) == 0 {
		// potentially untagged image of format <none>:<none>; remove by ID
		imageManager.deleteImage(ctx, leastRecentlyUsedImage.Image.ImageID, leastRecentlyUsedImage)
	} else {
		// Image has multiple tags/repos. Untag each name and delete the final reference to image
		for _, imageName := range imageNames {
			imageManager.deleteImage(ctx, imageName, leastRecentlyUsedImage)
		}
	}
}

func (imageManager *dockerImageManager) deleteImage(ctx context.Context, imageID string, imageState *image.ImageState) {
	if imageID == "" {
		var fields logger.Fields
		if imageState != nil {
			fields = imageState.Fields()
		}
		logger.Error("Image ID to be deleted is null", fields)
		return
	}
	logger.Debug(fmt.Sprintf("Removing Image: %s", imageID), imageState.Fields())
	err := imageManager.client.RemoveImage(ctx, imageID, dockerclient.RemoveImageTimeout)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), imageNotFoundForDeletionError) {
			logger.Error(fmt.Sprintf("Image already removed from the instance: %v", imageID), imageState.Fields())
		} else {
			logger.Error(fmt.Sprintf("Error removing Image %v", imageID), imageState.Fields(), logger.Fields{field.Error: err})
			delete(imageManager.imageStatesConsideredForDeletion, imageState.Image.ImageID)
			return
		}
	}
	logger.Info(fmt.Sprintf("Image removed: %v", imageID), imageState.Fields())
	imageState.RemoveImageName(imageID)
	if len(imageState.Image.Names) == 0 {
		logger.Info(fmt.Sprintf("Cleaning up all tracking information for image %s as it has zero references", imageID), imageState.Fields())
		delete(imageManager.imageStatesConsideredForDeletion, imageState.Image.ImageID)
		imageManager.removeImageState(imageState)
		imageManager.state.RemoveImageState(imageState)
	}
}

func (imageManager *dockerImageManager) GetImageStateFromImageName(containerImageName string) (*image.ImageState, bool) {
	imageManager.updateLock.Lock()
	defer imageManager.updateLock.Unlock()
	for _, imageState := range imageManager.getAllImageStates() {
		for _, imageName := range imageState.Image.Names {
			if imageName == containerImageName {
				return imageState, true
			}
		}
	}
	return nil, false
}
