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

const (
	// DefaultPortRangeStart indicates the first port in ephemeral port range
	DefaultPortRangeStart = 49153
	// DefaultPortRangeEnd indicates the last port in ephemeral port range
	DefaultPortRangeEnd = 65535
)

// GetDynamicHostPortRange returns the default ephemeral port range
func GetDynamicHostPortRange() (start int, end int, err error) {
	return DefaultPortRangeStart, DefaultPortRangeEnd, nil
}
