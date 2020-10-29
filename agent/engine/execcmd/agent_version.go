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

package execcmd

import (
	"errors"
	"strconv"
	"strings"
)

// TODO: Remove this later, just doing it do make static checks happy about these utils not being invoked anywhere
func init() {
	determineLatestVersion([]string{"1.1.1.1"})
}

const (
	versionDelimiter  = "."
	badInputReturn    = -999
	badFormatErrorStr = "invalid version format"
)

// compareAgentVersion compares two versions, returning -1 if v1 < v2, 0 if v1 == v2, and 1 if v1 > v2.
// The version must have only integers representing major, minor, patch, and reserved; delimited by dots (e.g. "1.2.3.4").
// An error is returned if any of the versions passed as parameter is malformed.
// example:
// compareAgentVersion("3.0.236.0", "3.0.237.0") returns -1, nil
// compareAgentVersion("3.0.236.0", "3.0.236.0") returns 0, nil
// compareAgentVersion("3.0.237.0", "3.0.236.0") returns 1, nil
// compareAgentVersion("a.b.c.d", "3.0.236.0") returns -999, error
func compareAgentVersion(v1Str, v2Str string) (int, error) {
	v1Arr := strings.Split(v1Str, versionDelimiter)
	v2Arr := strings.Split(v2Str, versionDelimiter)

	if len(v1Arr) != len(v2Arr) {
		return badInputReturn, errors.New(badFormatErrorStr)
	}
	for i := 0; i < len(v1Arr); i++ {
		v1Int, err := strconv.ParseUint(v1Arr[i], 10, 32)
		if err != nil {
			return badInputReturn, errors.New(badFormatErrorStr)
		}
		v2Int, err := strconv.ParseUint(v2Arr[i], 10, 32)
		if err != nil {
			return badInputReturn, errors.New(badFormatErrorStr)
		}

		if v1Int < v2Int {
			return -1, nil
		} else if v1Int > v2Int {
			return 1, nil
		}
	}
	return 0, nil
}

func determineLatestVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", errors.New("no versions to compare were provided")
	}
	latest := "0.0.0.0"
	for _, v := range versions {
		comp, err := compareAgentVersion(v, latest)
		if err != nil {
			return "", err
		}
		if comp > 0 {
			latest = v
		}
	}
	return latest, nil
}
