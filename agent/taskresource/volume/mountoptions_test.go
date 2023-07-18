//go:build unit
// +build unit

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVolumeMountOptions(t *testing.T) {
	assert.Equal(t, []string{"a", "b=c"}, NewVolumeMountOptions("a", "b=c").opts)
}

func TestNewVolumeMountOptionsFromString(t *testing.T) {
	assert.Equal(t, []string{"a", "b=c"}, NewVolumeMountOptionsFromString("a,b=c").opts)
	assert.Equal(t, []string{}, NewVolumeMountOptionsFromString("").opts)
}

func TestString(t *testing.T) {
	assert.Equal(t, "a", NewVolumeMountOptions("a").String())
	assert.Equal(t, "a,b=c", NewVolumeMountOptions("a", "b=c").String())
}

func TestAddOption(t *testing.T) {
	mntOpts := NewVolumeMountOptions()
	mntOpts.AddOption("a", "")
	assert.Equal(t, []string{"a"}, mntOpts.opts)
	mntOpts.AddOption("b", "c")
	assert.Equal(t, []string{"a", "b=c"}, mntOpts.opts)
}
