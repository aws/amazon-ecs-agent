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
	"fmt"
	"strings"
)

// Protection type to set on the task
type taskProtectionType string

const (
	TaskProtectionTypeScaleIn  taskProtectionType = "SCALE_IN"
	TaskProtectionTypeDisabled taskProtectionType = "DISABLED"
)

// Converts a string to taskProtectionType
func taskProtectionTypeFromString(s string) (taskProtectionType, error) {
	switch p := strings.ToUpper(s); p {
	case string(TaskProtectionTypeScaleIn):
		return TaskProtectionTypeScaleIn, nil
	case string(TaskProtectionTypeDisabled):
		return TaskProtectionTypeDisabled, nil
	default:
		return "", fmt.Errorf("unknown task protection type: %v", s)
	}
}

// Protection for a Task
type taskProtection struct {
	protectionType           taskProtectionType
	protectionTimeoutMinutes *int
}

// Creates a taskProtection value after validating the arguments
func NewTaskProtection(protectionType string, timeoutMinutes *int) (*taskProtection, error) {
	ptype, err := taskProtectionTypeFromString(protectionType)
	if err != nil {
		return nil, fmt.Errorf("protection type is invalid: %w", err)
	}

	if timeoutMinutes != nil && *timeoutMinutes <= 0 {
		return nil, fmt.Errorf("protection timeout must be greater than zero")
	}

	return &taskProtection{protectionType: ptype, protectionTimeoutMinutes: timeoutMinutes}, nil
}

func (taskProtection *taskProtection) GetProtectionType() taskProtectionType {
	return taskProtection.protectionType
}

func (taskProtection *taskProtection) GetProtectionTimeoutMinutes() *int {
	return taskProtection.protectionTimeoutMinutes
}
