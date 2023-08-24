//go:build windows && unit
// +build windows,unit

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

package resource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVolumeID = "vol-0a1234f3403277890"
)

func Test_parseExecutableOutput_HappyPath(t *testing.T) {
	output := fmt.Sprintf("SerialNumber \n	%s_00000001.", testVolumeID)
	parsedOutput, err := parseExecutableOutput([]byte(output))
	require.NoError(t, err)
	assert.True(t, strings.Contains(parsedOutput, testVolumeID))
}

func Test_parseExecutableOutput_UnexpectedOutput(t *testing.T) {
	output := "No Instance(s) Available."
	parsedOutput, err := parseExecutableOutput([]byte(output))
	require.Error(t, err, "cannot find the volume ID: %s", output)
	assert.Equal(t, "", parsedOutput)
}
