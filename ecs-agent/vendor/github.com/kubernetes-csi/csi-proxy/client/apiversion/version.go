/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package apiversion

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

// API version names follow k8s style version names:
// valid API version names examples: "v1", "v1alpha2", "v2beta3", etc...
var nameRegex = regexp.MustCompile("^v([1-9][0-9]*)(?:(alpha|beta)([1-9][0-9]*))?$")

type qualifier uint

const (
	alpha qualifier = iota
	beta
	stable
)

type Version struct {
	// major version number, eg 1 for "v1", 2 for "v2beta3"
	major uint

	// qualifier is "alpha" or "beta"
	qualifier qualifier

	// the qualifier's version, eg 2 for "v1alpha2" or 3 for "v2beta3"
	qualifierVersion uint

	rawName string
}

// NewVersion builds a Version from a version string, eg "v1alpha2"
func NewVersion(name string) (version Version, err error) {
	matches := nameRegex.FindStringSubmatch(name)
	if len(matches) < 2 || matches[1] == "" {
		err = fmt.Errorf("invalid version name: %q", name)
		return
	}

	major, err := toUint(matches[1], name)
	if err != nil {
		return
	}

	qualifier := stable
	var qualifierVersion uint
	if len(matches) >= 4 && matches[2] != "" {
		switch matches[2] {
		case "alpha":
			qualifier = alpha
		case "beta":
			qualifier = beta
		}

		qualifierVersion, err = toUint(matches[3], name)
		if err != nil {
			return
		}
	}

	version.major = major
	version.qualifier = qualifier
	version.qualifierVersion = qualifierVersion
	version.rawName = name

	return
}

// NewVersionOrPanic is the same as NewVersion, but panics if
// passed an invalid version name.
func NewVersionOrPanic(name string) Version {
	version, err := NewVersion(name)
	if err != nil {
		panic(err)
	}
	return version
}

// IsValidVersion returns true iff the input is a valid version name.
func IsValidVersion(name string) bool {
	return nameRegex.MatchString(name)
}

func toUint(s, name string) (uint, error) {
	i, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to convert %q from version name %q to int", s, name)
	}
	return uint(i), nil
}

type Comparison int

const (
	Lesser  Comparison = -1
	Equal   Comparison = 0
	Greater Comparison = 1
)

// Compare return Lesser if v < other (ie other is more recent), Equal if v == other,
// and Greater if v > other (ie v is more recent)
func (v Version) Compare(other Version) Comparison {
	if cmp := compareUints(v.major, other.major); cmp != Equal {
		return cmp
	}
	if cmp := compareUints(uint(v.qualifier), uint(other.qualifier)); cmp != Equal {
		return cmp
	}
	return compareUints(v.qualifierVersion, other.qualifierVersion)
}

func compareUints(a, b uint) Comparison {
	if a < b {
		return Lesser
	}
	if a > b {
		return Greater
	}
	return Equal
}

func (v Version) String() string {
	return v.rawName
}
