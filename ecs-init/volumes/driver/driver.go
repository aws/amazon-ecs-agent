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

package driver

import "github.com/aws/amazon-ecs-agent/ecs-init/volumes/types"

//go:generate mockgen.sh $GOPACKAGE_mock $GOFILE ./mock

// VolumeDriver contains the methods for volume drivers to implement
type VolumeDriver interface {
	// Setup is used to add a volume to the driver's state.
	// It should be called when the state of a volume is being loaded from storage.
	Setup(volumeName string, volume *types.Volume)

	// Create mounts the volume on the host.
	Create(createRequest *CreateRequest) error

	// Remove unmounts the volume from the host.
	Remove(removeRequest *RemoveRequest) error

	// Method to check if a volume is currently mounted.
	IsMounted(volumeName string) bool
}

// CreateRequest holds fields necessary for creating a volume
type CreateRequest struct {
	Name    string
	Path    string
	Options map[string]string
}

// RemoveRequest holds fields necessary for removing a volume
type RemoveRequest struct {
	Name string
}
