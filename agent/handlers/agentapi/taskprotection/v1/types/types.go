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
)

// Protection for a Task
type taskProtection struct {
	protectionEnabled bool
	expiresInMinutes  *int
}

// Custom JSON marshal function to marshal unexported fields for logging purposes
func (taskProtection *taskProtection) MarshalJSON() ([]byte, error) {
	jsonBytes, err := json.Marshal(struct {
		ProtectionEnabled bool
		ExpiresInMinutes  *int
	}{
		ProtectionEnabled: taskProtection.protectionEnabled,
		ExpiresInMinutes:  taskProtection.expiresInMinutes,
	})

	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

// Creates a taskProtection value after validating the arguments
func NewTaskProtection(protectionEnabled bool, expiresInMinutes *int) (*taskProtection, error) {
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

func (taskProtection *taskProtection) GetExpiresInMinutes() *int {
	return taskProtection.expiresInMinutes
}

func (taskProtection *taskProtection) String() string {
	jsonBytes, err := taskProtection.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("failed to get String representation of taskProtection type: %v", err)
	}
	return string(jsonBytes)
}
