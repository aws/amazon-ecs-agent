// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package util

import (
	"fmt"
	"strconv"
	"strings"
)

type Version string

func (lhs Version) Matches(selector string) (bool, error) {
	if strings.HasPrefix(selector, ">=") {
		if string(lhs) == selector[2:] {
			return true, nil
		}
		lessthan, err := lhs.lessThan(Version(selector[2:]))
		return !lessthan, err
	} else if strings.HasPrefix(selector, ">") {
		lessthan, err := lhs.lessThan(Version(selector[1:]))
		return !lessthan, err
	} else if strings.HasPrefix(selector, "<=") {
		if string(lhs) == selector[2:] {
			return true, nil
		}
		lessthan, err := lhs.lessThan(Version(selector[2:]))
		return lessthan, err
	} else if strings.HasPrefix(selector, "<") {
		lessthan, err := lhs.lessThan(Version(selector[1:]))
		return lessthan, err
	}
	lessthan, err := lhs.lessThan(Version(selector))
	return !lessthan, err
}

func (lhs Version) lessThan(rhs Version) (bool, error) {
	lhsParts := strings.Split(string(lhs), ".")
	if len(lhsParts) != 3 {
		return false, fmt.Errorf("Unrecognized version '%v'; must have two dots", lhs)
	}
	rhsParts := strings.Split(string(rhs), ".")
	if len(rhsParts) != 3 {
		return false, fmt.Errorf("Unrecognized version '%v'; must have two dots", rhs)
	}

	for i := 0; i < 3; i++ {
		lhsVal, err := strconv.Atoi(lhsParts[i])
		if err != nil {
			return false, fmt.Errorf("Invalid part of version '%s', part %s was not an integer", lhs, lhsParts[i])
		}
		rhsVal, err := strconv.Atoi(rhsParts[i])
		if err != nil {
			return false, fmt.Errorf("Invalid part of version '%s', part %s was not an integer", rhs, rhsParts[i])
		}
		if lhsVal > rhsVal {
			return false, nil
		}
		if rhsVal > lhsVal {
			return true, nil
		}
	}
	if lhs == rhs {
		return false, nil
	}
	return true, nil
}
