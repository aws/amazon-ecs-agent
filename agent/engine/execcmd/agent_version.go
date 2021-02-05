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
	"io/ioutil"
	"strconv"
	"strings"
)

const (
	versionDelimiter  = "."
	badFormatErrorStr = "invalid version format"
)

type agentVersion []uint

// compare compares this agentVersion against another one passed as parameter.
// returns -1 if lhs < rhs, 0 lhs == rhs, and 1 if lhs > rhs.
// example:
// [3, 0, 236, 0].compare([3, 0, 237, 0]) returns -1
// [3, 0, 236, 0].compare([3, 0, 236, 0]) returns 0
// [3, 0, 237, 0].compare([3, 0, 236, 0]) returns 1
// [3].compare([2, 0, 236])               returns 1
func (lhs agentVersion) compare(rhs agentVersion) int {
	lhsLen := len(lhs)
	rhsLen := len(rhs)
	maxLen := lhsLen
	if rhsLen > maxLen {
		maxLen = rhsLen
	}
	for i := 0; i < maxLen; i++ {
		v1 := uint(0)
		v2 := uint(0)
		if i < lhsLen {
			v1 = lhs[i]
		}
		if i < rhsLen {
			v2 = rhs[i]
		}
		if v1 < v2 {
			return -1
		} else if v1 > v2 {
			return 1
		}
	}
	return 0
}

// String the string representation of an agentVersion
// e.g. [3, 0, 236, 0] -> "3.0.236.0"
func (lhs agentVersion) String() string {
	var avStr []string
	for _, e := range lhs {
		avStr = append(avStr, strconv.Itoa(int(e)))
	}
	return strings.Join(avStr, versionDelimiter)
}

func parseAgentVersion(version string) (agentVersion, error) {
	var parsedVersion []uint
	vArr := strings.Split(version, versionDelimiter)
	for _, part := range vArr {
		pInt, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, errors.New(badFormatErrorStr)
		}
		parsedVersion = append(parsedVersion, uint(pInt))
	}
	return parsedVersion, nil
}

// This is a helper type to make []agentVersion sortable
type byAgentVersion []agentVersion

// Len fulfills sort.Interface
func (bav byAgentVersion) Len() int {
	return len(bav)
}

// Less fulfills sort.Interface
func (bav byAgentVersion) Less(i, j int) bool {
	return bav[i].compare(bav[j]) < 0
}

// Swap fulfills sort.Interface
func (bav byAgentVersion) Swap(i, j int) {
	bav[i], bav[j] = bav[j], bav[i]
}

var ioUtilReadDir = ioutil.ReadDir

func retrieveAgentVersions(agentBinPath string) ([]agentVersion, error) {
	files, err := ioUtilReadDir(agentBinPath)
	if err != nil {
		return nil, err
	}
	var versions []agentVersion
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		v, err := parseAgentVersion(f.Name())
		if err != nil { // The bin directory could have random folders, so we omit anything that cannot be parsed to a version
			continue
		}
		versions = append(versions, v)
	}
	return versions, nil
}
