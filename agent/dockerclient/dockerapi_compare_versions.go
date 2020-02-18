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
	"strconv"
	"strings"
)

type DockerAPIVersion string

type dockerVersion struct {
	major int
	minor int
}

// Matches returns whether or not a version matches a given selector.
// The selector can be any of the following:
//
// * x.y -- Matches a version exactly the same as the selector version
// * >=x.y -- Matches a version greater than or equal to the selector version
// * >x.y -- Matches a version greater than the selector version
// * <=x.y -- Matches a version less than or equal to the selector version
// * <x.y -- Matches a version less than the selector version
// * x.y,a.b -- Matches if the version matches either of the two selector versions
func (lhs DockerAPIVersion) Matches(selector string) (bool, error) {
	lhsVersion, err := parseDockerVersions(string(lhs))
	if err != nil {
		return false, err
	}

	if strings.Contains(selector, ",") {
		orElements := strings.Split(selector, ",")
		for _, el := range orElements {
			if elMatches, err := lhs.Matches(el); err != nil {
				return false, err
			} else if elMatches {
				return true, nil
			}
		}
		// No elements matched
		return false, nil
	}

	if strings.HasPrefix(selector, ">=") {
		rhsVersion, err := parseDockerVersions(selector[2:])
		if err != nil {
			return false, err
		}
		return compareDockerVersions(lhsVersion, rhsVersion) >= 0, nil
	} else if strings.HasPrefix(selector, ">") {
		rhsVersion, err := parseDockerVersions(selector[1:])
		if err != nil {
			return false, err
		}
		return compareDockerVersions(lhsVersion, rhsVersion) > 0, nil
	} else if strings.HasPrefix(selector, "<=") {
		rhsVersion, err := parseDockerVersions(selector[2:])
		if err != nil {
			return false, err
		}
		return compareDockerVersions(lhsVersion, rhsVersion) <= 0, nil
	} else if strings.HasPrefix(selector, "<") {
		rhsVersion, err := parseDockerVersions(selector[1:])
		if err != nil {
			return false, err
		}
		return compareDockerVersions(lhsVersion, rhsVersion) < 0, nil
	}

	rhsVersion, err := parseDockerVersions(selector)
	if err != nil {
		return false, err
	}
	return compareDockerVersions(lhsVersion, rhsVersion) == 0, nil
}

func parseDockerVersions(version string) (dockerVersion, error) {
	var result dockerVersion
	// x.x
	versionParts := strings.Split(version, ".")
	// [x, x]
	if len(versionParts) != 2 {
		return result, fmt.Errorf("Not enough '.' characters in the version part")
	}
	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return result, fmt.Errorf("Cannot parse major version as int: %v", err)
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return result, fmt.Errorf("Cannot parse minor version as int: %v", err)
	}
	result.major = major
	result.minor = minor
	return result, nil
}

// compareDockerVersions compares two docker api versions, 'lhs' and 'rhs', and returns -1 if lhs is less
// than rhs, 0 if they are equal, and 1 if lhs is greater than rhs
func compareDockerVersions(lhs, rhs dockerVersion) int {
	switch {
	case lhs.major < rhs.major:
		return -1
	case lhs.major > rhs.major:
		return 1
	case lhs.minor < rhs.minor:
		return -1
	case lhs.minor > rhs.minor:
		return 1
	}
	return 0
}
