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

package types

// Volume holds full details about a volume
type Volume struct {
	Type      string
	Path      string
	Options   map[string]string
	CreatedAt string
	Mounts    map[string]int
}

// Adds a new mount to the volume.
// This method is not thread-safe, caller is responsible for holding any locks on the volume.
func (v *Volume) AddMount(mountID string) {
	if v.Mounts == nil {
		v.Mounts = map[string]int{}
	}
	v.Mounts[mountID] += 1
}

// Removes a mount from the volume.
// This method is not thread-safe, caller is responsible for holding any locks on the volume.
// Returns a bool indicating whether the mountID was found in mounts or not.
func (v *Volume) RemoveMount(mountID string) bool {
	_, exists := v.Mounts[mountID]
	if !exists {
		return false
	}

	v.Mounts[mountID] -= 1
	if v.Mounts[mountID] <= 0 {
		delete(v.Mounts, mountID)
	}

	return true
}
