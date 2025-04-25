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

package userparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseUser(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectedUser       string
		expectedGroup      string
		wantErr            bool
		expectedErrMessage string
	}{
		{
			name:               "empty input",
			input:              "",
			wantErr:            true,
			expectedErrMessage: "empty user string provided",
		},
		{
			name:          "user only",
			input:         "foo",
			expectedUser:  "foo",
			expectedGroup: "",
		},
		{
			name:          "user and group",
			input:         "foo:bar",
			expectedUser:  "foo",
			expectedGroup: "bar",
		},
		{
			name:               "too many colons",
			input:              "dog:cat:cow",
			wantErr:            true,
			expectedErrMessage: "invalid format: expected 'user' or 'user:group'",
		},
		{
			name:               "just colon",
			input:              ":",
			wantErr:            true,
			expectedErrMessage: "invalid format: expected 'user' or 'user:group'",
		},
		{
			name:               "no user, but group present",
			input:              ":bar",
			wantErr:            true,
			expectedErrMessage: "invalid format: expected 'user' or 'user:group'",
		},
		{
			name:               "no group",
			input:              "foo:",
			wantErr:            true,
			expectedErrMessage: "invalid format: group part cannot be empty when using 'user:group' format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			userPart, groupPart, err := ParseUser(tc.input)
			assert.Equal(t, tc.wantErr, err != nil)
			if tc.wantErr {
				assert.Equal(t, tc.expectedErrMessage, err.Error())
			}
			assert.Equal(t, tc.expectedUser, userPart)
			assert.Equal(t, tc.expectedGroup, groupPart)
		})
	}
}
