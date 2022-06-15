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

package serviceconnect

import (
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	// serviceConnectConfigKey specifies the key maps to the service connect config in attachment properties
	serviceConnectConfigKey = "ServiceConnectConfig"
	// serviceConnectContainerNameKey specifies the key maps to the service connect container name in attachment properties
	serviceConnectContainerNameKey = "ContainerName"
	keyValidationMsgFormat         = `missing service connect config required key(s) in the attachment: found service connect config key: %t, found service connect container name key: %t`
)

func GetServiceConnectConfigKey() string {
	return serviceConnectConfigKey
}

func GetServiceConnectContainerNameKey() string {
	return serviceConnectContainerNameKey
}

// ParseServiceConnectAttachment parses the service connect container name and service connect config value
// from the given attachment.
func ParseServiceConnectAttachment(scAttachment *ecsacs.Attachment) (*Config, error) {
	scConfigValue := &Config{}
	containerName := ""
	foundSCConfigKey := false
	foundSCContainerNameKey := false

	for _, property := range scAttachment.AttachmentProperties {
		switch aws.StringValue(property.Name) {
		case serviceConnectConfigKey:
			foundSCConfigKey = true
			// extract service connect config value from the attachment property,
			// and translate the attachment property value to Config
			data := aws.StringValue(property.Value)
			if err := json.Unmarshal([]byte(data), scConfigValue); err != nil {
				return nil, fmt.Errorf("failed to unmarshal service connect attachment property value: %w", err)
			}
		case serviceConnectContainerNameKey:
			foundSCContainerNameKey = true
			// extract service connect container name from the attachment property
			containerName = aws.StringValue(property.Value)
		default:
			logger.Warn("Received an unrecognized attachment property", logger.Fields{
				"attachmentProperty": property.String(),
			})
		}
	}

	// returns error if service connect config or container name key does not exist
	if !foundSCConfigKey || !foundSCContainerNameKey {
		return nil, fmt.Errorf(keyValidationMsgFormat, foundSCConfigKey, foundSCContainerNameKey)
	}

	scConfigValue.ContainerName = containerName

	return scConfigValue, nil
}
