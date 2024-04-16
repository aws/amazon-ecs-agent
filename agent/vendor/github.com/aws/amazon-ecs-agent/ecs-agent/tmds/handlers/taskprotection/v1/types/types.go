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

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
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

// NewTaskProtection creates a taskProtection
func NewTaskProtection(protectionEnabled bool, expiresInMinutes *int64) *taskProtection {
	return &taskProtection{
		protectionEnabled: protectionEnabled,
		expiresInMinutes:  expiresInMinutes,
	}
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
	RequestID  *string            `json:"requestID,omitempty"`
	Protection *ecs.ProtectedTask `json:"protection,omitempty"`
	Failure    *ecs.Failure       `json:"failure,omitempty"`
	Error      *ErrorResponse     `json:"error,omitempty"`
}

// NewTaskProtectionResponseProtection creates a TaskProtectionResponse when it is a successful response (has protection)
func NewTaskProtectionResponseProtection(protection *ecs.ProtectedTask) TaskProtectionResponse {
	return TaskProtectionResponse{Protection: protection}
}

// NewTaskProtectionResponseFailure creates a TaskProtectionResponse when there is a failed response with failure
func NewTaskProtectionResponseFailure(failure *ecs.Failure) TaskProtectionResponse {
	return TaskProtectionResponse{Failure: failure}
}

// NewTaskProtectionResponseError creates a TaskProtectionResponse when there is an error response with optional requestID
func NewTaskProtectionResponseError(error *ErrorResponse, requestID *string) TaskProtectionResponse {
	return TaskProtectionResponse{RequestID: requestID, Error: error}
}

// ErrorResponse is the type for all Update/GetTaskProtection request errors
type ErrorResponse struct {
	Arn     string `json:"Arn,omitempty"`
	Code    string
	Message string
}

// NewErrorResponsePtr creates a *ErrorResponse for Agent input validations failures and exceptions
func NewErrorResponsePtr(arn string, code string, message string) *ErrorResponse {
	return &ErrorResponse{
		Arn:     arn,
		Code:    code,
		Message: message,
	}
}
