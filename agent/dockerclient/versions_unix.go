//go:build !windows
// +build !windows

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

package dockerclient

import "github.com/aws/amazon-ecs-agent/ecs-agent/logger"

// SupportedVersionsExtended takes in a function that returns supported docker API versions.
// It returns a list of "extended" API versions that may not be directly supported
// but their functionality is supported.
// For example, in Docker Engine 25.x API versions 1.17-1.23 were deprecated to be used
// directly, although their functionality is still supported.
func SupportedVersionsExtended(supportedVersionsFn func() []DockerVersion) []DockerVersion {
	supportedAPIVersions := supportedVersionsFn()
	// Check if MinDockerAPIVersion is greater than or equal to 1.24
	// This version is used because this is the earliest API version supported when docker
	// deprecated early API versions. See https://docs.docker.com/engine/deprecated/
	cmp := MinDockerAPIVersion.Compare(Version_1_24)

	if cmp == 1 || cmp == 0 {
		// extend list of supported versions to include earlier known versions of docker api
		extendedAPIVersions := []DockerVersion{}
		knownAPIVersions := GetKnownAPIVersions()
		for _, knownAPIVersion := range knownAPIVersions {
			if knownAPIVersion.Compare(MinDockerAPIVersion) < 0 {
				extendedAPIVersions = append(extendedAPIVersions, knownAPIVersion)
			}
		}
		supportedAPIVersions = append(extendedAPIVersions, supportedAPIVersions...)
	}

	logger.Debug("Extended supported docker API versions", logger.Fields{
		"supportedAPIVersions": supportedAPIVersions,
	})
	return supportedAPIVersions
}
