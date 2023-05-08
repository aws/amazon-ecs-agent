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
	"sort"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

const (
	appnetInterfaceV1          = "v1"
	serviceConnectCapabilityV1 = "ecs.capability.service-connect-v1"
)

var (
	supportedAppnetInterfaceVerToCapability = map[string][]string{
		appnetInterfaceV1: {
			serviceConnectCapabilityV1,
		},
	}
)

// getSupportedAppnetInterfaceVersions returns the all the supported AppNet interface versions
// by ECS Agent from highest to lowest version
func getSupportedAppnetInterfaceVersions() []string {
	supportedAppnetInterfaceVersions := make([]string, 0, len(supportedAppnetInterfaceVerToCapability))
	for supportedAppnetInterfaceVersion := range supportedAppnetInterfaceVerToCapability {
		supportedAppnetInterfaceVersions = append(supportedAppnetInterfaceVersions, supportedAppnetInterfaceVersion)
	}
	sort.Slice(
		supportedAppnetInterfaceVersions,
		func(i, j int) bool {
			return getVersionNumber(supportedAppnetInterfaceVersions[i]) > getVersionNumber(supportedAppnetInterfaceVersions[j])
		},
	)
	return supportedAppnetInterfaceVersions
}

// getVersionNumber returns a version sort key in numeric order.
func getVersionNumber(version string) int {
	versionNumber := strings.TrimPrefix(version, "v")
	num, err := strconv.Atoi(versionNumber)
	if err != nil {
		logger.Error("Error parsing appnet interface version number as int:", logger.Fields{
			field.Error: err,
		})
	}
	return num
}
