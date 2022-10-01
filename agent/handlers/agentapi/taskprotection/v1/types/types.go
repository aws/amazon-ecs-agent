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

import (
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
)

// taskProtection is type of Protection for a Task
type taskProtection struct {
	protectionEnabled bool
	expiresInMinutes  *int64
}

// MarshalJSON is custom JSON marshal function to marshal unexported fields for logging purposes
func (taskProtection *taskProtection) MarshalJSON() ([]byte, error) {
	jsonBytes, err := json.Marshal(struct {
		ProtectionEnabled bool
		ExpiresInMinutes  *int64
	}{
		ProtectionEnabled: taskProtection.protectionEnabled,
		ExpiresInMinutes:  taskProtection.expiresInMinutes,
	})

	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

// NewTaskProtection creates a taskProtection value after validating the arguments
func NewTaskProtection(protectionEnabled bool, expiresInMinutes *int64) (*taskProtection, error) {
	if protectionEnabled && expiresInMinutes != nil && *expiresInMinutes <= 0 {
		return nil, fmt.Errorf("expiration duration must be greater than zero minutes for enabled task protection")
	}

	return &taskProtection{
		protectionEnabled: protectionEnabled,
		expiresInMinutes:  expiresInMinutes,
	}, nil
}

func (taskProtection *taskProtection) GetProtectionEnabled() bool {
	return taskProtection.protectionEnabled
}

func (taskProtection *taskProtection) GetExpiresInMinutes() *int64 {
	return taskProtection.expiresInMinutes
}

func (taskProtection *taskProtection) String() string {
	jsonBytes, err := taskProtection.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("failed to get string representation of taskProtection type: %v", err)
	}
	return string(jsonBytes)
}

// TaskProtectionResponse is response type for all Update/GetTaskProtection requests
type TaskProtectionResponse struct {
	Protection *ecs.ProtectedTask `json:"protection,omitempty"`
	Failure    *ecs.Failure       `json:"failure,omitempty"`
}

// MarshalJSON is custom JSON marshal function to marshal unexported fields for logging purposes
func (taskProtectionResponse *TaskProtectionResponse) MarshalJSON() ([]byte, error) {
	jsonBytes, err := json.Marshal(struct {
		Protection ecs.ProtectedTask
		Failure    ecs.Failure
	}{
		Protection: *taskProtectionResponse.Protection,
		Failure:    *taskProtectionResponse.Failure,
	})

	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

// NewTaskProtectionResponse creates a taskProtection value after validating the arguments
func NewTaskProtectionResponse(protection *ecs.ProtectedTask, failure *ecs.Failure) TaskProtectionResponse {
	return TaskProtectionResponse{
		Protection: protection,
		Failure:    failure,
	}
}

func (taskProtectionResponse *TaskProtectionResponse) String() string {
	jsonBytes, err := taskProtectionResponse.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("failed to get string representation of taskProtectionResponse type: %v", err)
	}
	return string(jsonBytes)
}
