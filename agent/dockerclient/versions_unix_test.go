//go:build !windows && unit
// +build !windows,unit

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func setTestMinDockerAPIVersion(minVersion DockerVersion) func() {
	origMin := MinDockerAPIVersion
	SetMinDockerAPIVersion(minVersion)
	return func() {
		SetMinDockerAPIVersion(origMin)
	}
}

func TestSupportedVersionsExtended(t *testing.T) {
	defer setTestMinDockerAPIVersion(Version_1_19)()
	supportedVersionsTestFunc := func() []DockerVersion {
		return []DockerVersion{
			Version_1_19,
			Version_1_20,
			Version_1_21,
			Version_1_22,
			Version_1_23,
			Version_1_24,
		}
	}
	actualSupportedVersions := SupportedVersionsExtended(supportedVersionsTestFunc)
	expectedSupportedVersions := []DockerVersion{
		Version_1_19,
		Version_1_20,
		Version_1_21,
		Version_1_22,
		Version_1_23,
		Version_1_24,
	}

	assert.Equal(t,
		expectedSupportedVersions,
		actualSupportedVersions,
	)
}

func TestSupportedVersionsExtended_1_24(t *testing.T) {
	// Min version 1.24 triggers the 'extended' docker versions to be added
	defer setTestMinDockerAPIVersion(Version_1_24)()
	supportedVersionsTestFunc := func() []DockerVersion {
		return []DockerVersion{
			Version_1_24,
			Version_1_25,
			Version_1_26,
		}
	}
	actualSupportedVersions := SupportedVersionsExtended(supportedVersionsTestFunc)
	expectedSupportedVersions := []DockerVersion{
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
	}

	assert.Equal(t,
		expectedSupportedVersions,
		actualSupportedVersions,
	)
}

func TestSupportedVersionsExtended_1_26(t *testing.T) {
	// Min version greater thanm 1.24 triggers the 'extended' docker versions to be added
	defer setTestMinDockerAPIVersion(Version_1_26)()
	supportedVersionsTestFunc := func() []DockerVersion {
		return []DockerVersion{
			Version_1_26,
			Version_1_27,
		}
	}
	actualSupportedVersions := SupportedVersionsExtended(supportedVersionsTestFunc)
	expectedSupportedVersions := []DockerVersion{
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
	}

	assert.Equal(t,
		expectedSupportedVersions,
		actualSupportedVersions,
	)
}
