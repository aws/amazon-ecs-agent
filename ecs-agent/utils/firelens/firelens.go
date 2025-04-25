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

package firelens

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/userparser"
)

var osChown = os.Chown

// SetOwnership parses the user ID and group name/ID from the 'user' parameter and sets ownership for the given directory.
//
// The Firelens user must be one of the following formats:
//   - userID
//   - userID:groupID
//   - userID:groupName
//
// For userID:groupID format, we set the directory ownership using the given UID and GID.
// For userID and userID:groupName formats, we use the given UID, and the agent GID while assigning ownership.
func SetOwnership(directory, user string) error {
	if user == "" {
		logger.Debug("No user input specified, will not change ownership", logger.Fields{
			"FirelensDirectory": directory,
		})
		return nil
	}
	var userPart, groupPart string
	var err error

	// Parse the input user if provided
	userPart, groupPart, err = userparser.ParseUser(user)
	if err != nil {
		return fmt.Errorf("unable to parse Firelens user: %w", err)
	}

	// Convert user string to UID
	userID, err := strconv.Atoi(userPart)
	if err != nil {
		return fmt.Errorf("unable to determine the user ID: %w", err)
	}

	// Convert group string to GID
	var groupID int
	if groupPart == "" {
		logger.Debug("No group specified, will apply agent's group ownership", logger.Fields{
			"FirelensDirectory": directory,
		})
		groupID = os.Getgid()
	} else {
		groupID, err = strconv.Atoi(groupPart)
		if err != nil {
			logger.Debug("Specified group is not an ID, will assign ownership to agent's group ID", logger.Fields{
				"FirelensDirectory": directory,
			})
			groupID = os.Getgid()
		}
	}

	// Set ownership of the given directory
	if err := osChown(directory, userID, groupID); err != nil {
		return fmt.Errorf("unable to set directory ownership: %w", err)
	}
	return nil
}
