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

import (
	"fmt"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

type DockerVersion string

var (
	// MinDockerAPIVersion is the min Docker API version supported by agent
	MinDockerAPIVersion   = Version_1_21
	MinDockerAPIVersionMu sync.Mutex
)

const (
	Version_1_17 DockerVersion = "1.17"
	Version_1_18 DockerVersion = "1.18"
	Version_1_19 DockerVersion = "1.19"
	Version_1_20 DockerVersion = "1.20"
	Version_1_21 DockerVersion = "1.21"
	Version_1_22 DockerVersion = "1.22"
	Version_1_23 DockerVersion = "1.23"
	Version_1_24 DockerVersion = "1.24"
	Version_1_25 DockerVersion = "1.25"
	Version_1_26 DockerVersion = "1.26"
	Version_1_27 DockerVersion = "1.27"
	Version_1_28 DockerVersion = "1.28"
	Version_1_29 DockerVersion = "1.29"
	Version_1_30 DockerVersion = "1.30"
	Version_1_31 DockerVersion = "1.31"
	Version_1_32 DockerVersion = "1.32"
	Version_1_33 DockerVersion = "1.33"
	Version_1_34 DockerVersion = "1.34"
	Version_1_35 DockerVersion = "1.35"
	Version_1_36 DockerVersion = "1.36"
	Version_1_37 DockerVersion = "1.37"
	Version_1_38 DockerVersion = "1.38"
	Version_1_39 DockerVersion = "1.39"
	Version_1_40 DockerVersion = "1.40"
	Version_1_41 DockerVersion = "1.41"
	Version_1_42 DockerVersion = "1.42"
	Version_1_43 DockerVersion = "1.43"
	Version_1_44 DockerVersion = "1.44"
)

func (d DockerVersion) String() string {
	return string(d)
}

// Compare returns 0 if versions are equal
// returns 1 if this version is greater than rhs
// returns -1 if this version if less than rhs
func (d DockerVersion) Compare(rhs DockerVersion) int {
	if d == rhs {
		return 0
	}
	dockerApiVersion := DockerAPIVersion(d)
	lessThan, err := dockerApiVersion.Matches(fmt.Sprintf("<%s", rhs))
	if err != nil {
		return 0
	}
	if lessThan {
		return -1
	}
	return 1
}

// SetMinDockerAPIVersion sets the minimum/default docker API version that the
// ECS agent will use.
func SetMinDockerAPIVersion(v DockerVersion) {
	MinDockerAPIVersionMu.Lock()
	defer MinDockerAPIVersionMu.Unlock()
	if MinDockerAPIVersion == v {
		return
	}
	logger.Info("Setting minimum docker API version", logger.Fields{
		"previousMinAPIVersion": MinDockerAPIVersion,
		"newMinAPIVersion":      v,
	})
	MinDockerAPIVersion = v
}

// GetSupportedDockerAPIVersion takes in a DockerVersion and:
// 1. if the input version is supported, then the same version is returned.
// 2. if the input version is less than the minimum supported version, then the minimum supported version is returned.
// 3. if the input version is invalid, log a warning and return the minimum supported version.
func GetSupportedDockerAPIVersion(version DockerVersion) DockerVersion {
	cmp := version.Compare(MinDockerAPIVersion)
	if cmp == 1 {
		return version
	}
	return MinDockerAPIVersion
}

// GetKnownAPIVersions returns all of the API versions that we know about.
// It doesn't care if the version is supported by Docker or ECS agent
func GetKnownAPIVersions() []DockerVersion {
	return []DockerVersion{
		Version_1_17,
		Version_1_18,
		Version_1_19,
		Version_1_20,
		Version_1_21,
		Version_1_22,
		Version_1_23,
		Version_1_24,
		Version_1_25,
		Version_1_26,
		Version_1_27,
		Version_1_28,
		Version_1_29,
		Version_1_30,
		Version_1_31,
		Version_1_32,
		Version_1_33,
		Version_1_34,
		Version_1_35,
		Version_1_36,
		Version_1_37,
		Version_1_38,
		Version_1_39,
		Version_1_40,
		Version_1_41,
		Version_1_42,
		Version_1_43,
		Version_1_44,
	}
}
