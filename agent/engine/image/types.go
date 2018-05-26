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

package image

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/cihub/seelog"
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
	Containers []*api.Container `json:"-"`
	// PulledAt is the time when this image was pulled.
	PulledAt time.Time
	// LastUsedAt is the time when this image was used last time.
	LastUsedAt time.Time
	// PullSucceeded defines whether this image has been pulled successfully before,
	// this should be set to true when one of the pull image call succeeds.
	PullSucceeded bool
	lock          sync.RWMutex
}

func (imageState *ImageState) UpdateContainerReference(container *api.Container) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	seelog.Infof("Updating container reference %v in Image State - %v", container.Name, imageState.Image.ImageID)
	imageState.Containers = append(imageState.Containers, container)
}

func (imageState *ImageState) AddImageName(imageName string) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	if !imageState.HasImageName(imageName) {
		seelog.Infof("Adding image name- %v to Image state- %v", imageName, imageState.Image.ImageID)
		imageState.Image.Names = append(imageState.Image.Names, imageName)
	}
}

func (imageState *ImageState) GetImageNamesCount() int {
	imageState.lock.RLock()
	defer imageState.lock.RUnlock()
	return len(imageState.Image.Names)
}

func (imageState *ImageState) HasNoAssociatedContainers() bool {
	return len(imageState.Containers) == 0
}

func (imageState *ImageState) UpdateImageState(container *api.Container) {
	imageState.AddImageName(container.Image)
	imageState.UpdateContainerReference(container)
}

func (imageState *ImageState) RemoveImageName(containerImageName string) {
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	for i, imageName := range imageState.Image.Names {
		if imageName == containerImageName {
			imageState.Image.Names = append(imageState.Image.Names[:i], imageState.Image.Names[i+1:]...)
		}
	}
}

func (imageState *ImageState) HasImageName(containerImageName string) bool {
	for _, imageName := range imageState.Image.Names {
		if imageName == containerImageName {
			return true
		}
	}
	return false
}

func (imageState *ImageState) RemoveContainerReference(container *api.Container) error {
	// Get the image state write lock for updating container reference
	imageState.lock.Lock()
	defer imageState.lock.Unlock()
	for i := range imageState.Containers {
		if imageState.Containers[i].Name == container.Name {
			// Container reference found; hence remove it
			seelog.Infof("Removing Container Reference: %v from Image State- %v", container.Name, imageState.Image.ImageID)
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
