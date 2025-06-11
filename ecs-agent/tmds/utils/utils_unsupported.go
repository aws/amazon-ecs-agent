//go:build !linux && !windows
// +build !linux,!windows

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
package utils

import (
	"errors"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
)

// GetDefaultNetworkInterfaces returns network interfaces with the highest priority default routes
// for IPv4 and IPv6. If both default routes use the same interface, only one interface is returned.
// Returns an empty slice if no default routes are found. This is only supported on linux as of now.
func GetDefaultNetworkInterfaces(
	unknown interface{},
) ([]state.NetworkInterface, error) {
	return nil, errors.New("not supported on unknown platform")
}
