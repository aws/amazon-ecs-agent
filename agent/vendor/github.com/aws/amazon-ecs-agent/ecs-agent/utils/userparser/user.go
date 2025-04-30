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

package userparser

import (
	"fmt"
	"strings"
)

// ParseUser takes a user string and parses it into user and group components.
// If no group is specified, the group returned will be empty.
// Returns an error if the input string is empty or contains more than one colon separator.
func ParseUser(user string) (string, string, error) {
	if user == "" {
		return "", "", fmt.Errorf("empty user string provided")
	}

	// Split on colon to separate user and group
	parts := strings.Split(user, ":")
	// Validate number of parts
	if len(parts) > 2 {
		return "", "", fmt.Errorf("invalid format: expected 'user' or 'user:group'")
	}

	// Get user part
	userPart := parts[0]
	if userPart == "" {
		return "", "", fmt.Errorf("invalid format: expected 'user' or 'user:group'")
	}

	// Get group part if provided, empty string if not
	groupPart := ""
	if len(parts) == 2 {
		groupPart = parts[1]
		if groupPart == "" {
			return "", "", fmt.Errorf("invalid format: group part cannot be empty when using 'user:group' format")
		}
	}
	return userPart, groupPart, nil
}
