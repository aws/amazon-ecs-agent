package firelens

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/userparser"
)

// SetOwnership parses the user ID and group name/ID from the 'user' parameter and sets ownership for the given directory.
//
// The Firelens user must be one of the following formats:
//   - userID
//   - userID:groupID
//   - userID:groupName
//
// For userID:groupID format, we set the directory ownership using the given UID and GID.
// For userID and userID:groupName formats, we use given UID, and the agent GID while assigning ownership.
func SetOwnership(directory, user string) error {
	var userPart, groupPart string
	var err error

	// Parse the input user if provided
	if user != "" {
		userPart, groupPart, err = userparser.ParseUser(user)
		if err != nil {
			return fmt.Errorf("unable to parse Firelens user: %w", err)
		}
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
			logger.Debug("Specified group is not an ID, will apply agent's group ownership", logger.Fields{
				"FirelensDirectory": directory,
			})
			groupID = os.Getgid()
		}
	}

	// Set ownership of the given directory
	if err := os.Chown(directory, userID, groupID); err != nil {
		return fmt.Errorf("unable to set directory ownership: %w", err)
	}
	return nil
}
