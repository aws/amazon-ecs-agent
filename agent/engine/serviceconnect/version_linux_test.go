//go:build linux && unit
// +build linux,unit

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
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

const appNetInterfaceVersionPattern = "^v(\\d*)$"

func TestSupportedAppnetInterfaceVersion(t *testing.T) {
	allowedVersionPattern, err := regexp.Compile(appNetInterfaceVersionPattern)
	for supportedAppnetInterfaceVersion := range supportedAppnetInterfaceVerToCapability {
		require.NoError(t, err, "Error while compiling regex pattern")
		require.True(t, allowedVersionPattern.MatchString(supportedAppnetInterfaceVersion),
			"Appnet interface version does not match the expected pattern, ex:`v1`")
	}
}
func TestGetSupportedAppnetInterfaceVersion(t *testing.T) {
	tempSupportedAppnetInterfaceVerToCapability := supportedAppnetInterfaceVerToCapability
	defer func() {
		supportedAppnetInterfaceVerToCapability = tempSupportedAppnetInterfaceVerToCapability
	}()

	supportedAppnetInterfaceVerToCapability = map[string][]string{
		"v1": {
			"ecs.capability.service-connect-v1",
		},
		"v2": {
			"ecs.capability.service-connect-v1",
			"ecs.capability.service-connect-v2",
		},
		"v11": {
			"ecs.capability.service-connect-v10",
			"ecs.capability.service-connect-v11",
		},
		"v10": {
			"ecs.capability.service-connect-v10",
		},
	}

	expectedSupportedAppnetInterfaceVersions := []string{"v11", "v10", "v2", "v1"}
	require.Equal(t, expectedSupportedAppnetInterfaceVersions, getSupportedAppnetInterfaceVersions())
}
