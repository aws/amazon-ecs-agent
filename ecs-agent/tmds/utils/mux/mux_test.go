//go:build unit
// +build unit

// copyright amazon.com inc. or its affiliates. all rights reserved.
//
// licensed under the apache license, version 2.0 (the "license"). you may
// not use this file except in compliance with the license. a copy of the
// license is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. this file is distributed
// on an "as is" basis, without warranties or conditions of any kind, either
// express or implied. see the license for the specific language governing
// permissions and limitations under the license.
package mux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstructMuxVar(t *testing.T) {
	testCases := []struct {
		testName    string
		name        string
		pattern     string
		expectedVar string
	}{
		{
			testName:    "With anything but slash pattern",
			name:        "muxname",
			pattern:     AnythingButSlashRegEx,
			expectedVar: "{muxname:[^/]*}",
		},
		{
			testName:    "With anything pattern",
			name:        "muxname",
			pattern:     AnythingRegEx,
			expectedVar: "{muxname:.*}",
		},
		{
			testName:    "With anything but empty pattern",
			name:        "muxname",
			pattern:     AnythingButEmptyRegEx,
			expectedVar: "{muxname:.+}",
		},
		{
			testName:    "Without pattern",
			name:        "muxname",
			pattern:     "",
			expectedVar: "{muxname}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			assert.Equal(t, tc.expectedVar, ConstructMuxVar(tc.name, tc.pattern))
		})
	}
}
