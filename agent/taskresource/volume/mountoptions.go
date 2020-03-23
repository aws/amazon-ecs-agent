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

package volume

import (
	"strings"
)

const mntOptSep = ","

// VolumeMountOptions contains a list of mount options for mounting a volume.
// This struct exists mainly to make serializing/deserializing a list of options easier.
type VolumeMountOptions struct {
	opts []string
}

// NewVolumeMountOptions returns a new VolumeMountOptions with options from argument.
func NewVolumeMountOptions(opts ...string) *VolumeMountOptions {
	return &VolumeMountOptions{
		opts: opts,
	}
}

// NewVolumeMountOptionsFromString returns a new VolumeMountOptions with options from argument.
func NewVolumeMountOptionsFromString(s string) *VolumeMountOptions {
	opts := []string{}
	if s != "" { // s == "" needs to be dealt with specially because it ends up with an empty option string.
		opts = strings.Split(s, mntOptSep)
	}
	return &VolumeMountOptions{
		opts: opts,
	}
}

// String serializes a list of options joined by comma.
func (mo *VolumeMountOptions) String() string {
	return strings.Join(mo.opts, mntOptSep)
}

// AddOption adds a new option to the option list.
func (mo *VolumeMountOptions) AddOption(key, val string) {
	opt := key
	if val != "" {
		opt = opt + "=" + val
	}
	mo.opts = append(mo.opts, opt)
}
