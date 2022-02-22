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

package image

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
)

type Image struct {
	ImageID string
	Names   []string
	Size    int64
}

func (image *Image) String() string {
	return fmt.Sprintf("ImageID: %s; Names: %s", image.ImageID, strings.Join(image.Names, ", "))
}

// ImageState represents a docker image
// and its state information such as containers associated with it
type ImageState struct {
	// Image is the image corresponding to this ImageState.
	Image *Image
	// Containers are the containers that use this image.
	Containers []*apicontainer.Container `json:"-"`
	// PulledAt is the time when this image was pulled.
	PulledAt time.Time
	// LastUsedAt is the time when this image was used last time.
	LastUsedAt time.Time
	// PullSucceeded defines whether this image has been pulled successfully before,
	// this should be set to true when one of the pull image call succeeds.
	PullSucceeded bool
	lock          sync.RWMutex
}

// UpdateContainerReference updates container reference in image state
func (imageState *ImageState) UpdateContainerReference(container *apicontainer.Container) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	logger.Info("Updating container reference in image state", logger.Fields{
		field.Container: container.Name,
		field.Image:     container.Image,
		"state":         imageState.Image.ImageID,
	})
	imageState.Containers = append(imageState.Containers, container)
}

// AddImageName adds image name to image state
func (imageState *ImageState) AddImageName(imageName string) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	if !imageState.HasImageName(imageName) {
		logger.Info("Adding image to state", logger.Fields{
			field.Image: imageName,
			"state":     imageState.Image.ImageID,
		})
		imageState.Image.Names = append(imageState.Image.Names, imageName)
	}
}

// GetImageID returns id of image
func (imageState *ImageState) GetImageID() string {
	imageState.lock.RLock()
	defer imageState.lock.RUnlock()
	return imageState.Image.ImageID
}

// GetImageNamesCount returns number of image names
func (imageState *ImageState) GetImageNamesCount() int {
	imageState.lock.RLock()
	defer imageState.lock.RUnlock()
	return len(imageState.Image.Names)
}

// HasNoAssociatedContainers returns true if image has no associated containers, false otherwise
func (imageState *ImageState) HasNoAssociatedContainers() bool {
	return len(imageState.Containers) == 0
}

// UpdateImageState updates image name and container reference in image state
func (imageState *ImageState) UpdateImageState(container *apicontainer.Container) {
	imageState.AddImageName(container.Image)
	imageState.UpdateContainerReference(container)
}

// RemoveImageName removes image name from image state
func (imageState *ImageState) RemoveImageName(containerImageName string) bool {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	for i, imageName := range imageState.Image.Names {
		if imageName == containerImageName {
			imageState.Image.Names = append(imageState.Image.Names[:i], imageState.Image.Names[i+1:]...)
			return true
		}
	}
	return false
}

// HasImageName returns true if image state contains the containerImageName
func (imageState *ImageState) HasImageName(containerImageName string) bool {
	for _, imageName := range imageState.Image.Names {
		if imageName == containerImageName {
			return true
		}
	}
	return false
}

// RemoveContainerReference removes container reference from image state
func (imageState *ImageState) RemoveContainerReference(container *apicontainer.Container) error {
	// Get the image state write lock for updating container reference
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	for i := range imageState.Containers {
		if imageState.Containers[i].Name == container.Name {
			// Container reference found; hence remove it
			logger.Info("Removing container reference from image state", logger.Fields{
				field.Container: container.Name,
				field.Image:     container.Image,
				"state":         imageState.Image.ImageID,
			})
			imageState.Containers = append(imageState.Containers[:i], imageState.Containers[i+1:]...)
			// Update the last used time for the image
			imageState.LastUsedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("Container reference is not found in the image state container: %s", container.String())
}

// SetPullSucceeded sets the PullSucceeded of the imageState
func (imageState *ImageState) SetPullSucceeded(pullSucceeded bool) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()

	imageState.PullSucceeded = pullSucceeded
}

// GetPullSucceeded safely returns the PullSucceeded of the imageState
func (imageState *ImageState) GetPullSucceeded() bool {
	imageState.lock.RLock()
	defer imageState.lock.RUnlock()

	return imageState.PullSucceeded
}

// MarshalJSON marshals image state
func (imageState *ImageState) MarshalJSON() ([]byte, error) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()

	return json.Marshal(&struct {
		Image         *Image
		PulledAt      time.Time
		LastUsedAt    time.Time
		PullSucceeded bool
	}{
		Image:         imageState.Image,
		PulledAt:      imageState.PulledAt,
		LastUsedAt:    imageState.LastUsedAt,
		PullSucceeded: imageState.PullSucceeded,
	})
}

func (imageState *ImageState) String() string {
	image := ""
	if imageState.Image != nil {
		image = imageState.Image.String()
	}
	return fmt.Sprintf("Image: [%s] referenced by %d containers; PulledAt: %s; LastUsedAt: %s; PullSucceeded: %t",
		image, len(imageState.Containers), imageState.PulledAt.String(), imageState.LastUsedAt.String(), imageState.PullSucceeded)
}
