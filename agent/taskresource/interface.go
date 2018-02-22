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

package taskresource

import (
	"encoding/json"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

// ResourceStatus is an enumeration of valid states of task resource lifecycle
type ResourceStatus int32

// TaskResource is a wrapper for task level resource methods we need
type TaskResource interface {
	// SetDesiredStatus sets the desired status of the resource
	SetDesiredStatus(ResourceStatus)
	// GetDesiredStatus gets the desired status of the resource
	GetDesiredStatus() ResourceStatus
	// SetKnownStatus sets the desired status of the resource
	SetKnownStatus(ResourceStatus)
	// GetKnownStatus gets the desired status of the resource
	GetKnownStatus() ResourceStatus
	// SetCreatedAt sets the timestamp for resource's creation time
	SetCreatedAt(time.Time)
	// GetCreatedAt sets the timestamp for resource's creation time
	GetCreatedAt() time.Time
	// Create performs resource creation
	Create(task *api.Task) error
	// Cleanup performs resource cleanup
	Cleanup() error

	json.Marshaler
	json.Unmarshaler
}
