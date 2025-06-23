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

package firelens

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testDirectory           = "/foo"
	testInvalidDirectoryErr = "dir does not exist"
	testCurrGID             = os.Getgid()
)

func TestSetOwnership(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantErr          bool
		expectedErr      string
		expectedUID      int
		expectedGID      int
		invalidDirectory bool
	}{
		{
			name:    "Empty user string",
			input:   "",
			wantErr: false,
		},
		{
			name:        "Valid userID format",
			input:       "1234",
			wantErr:     false,
			expectedUID: 1234,
			expectedGID: testCurrGID,
		},
		{
			name:        "Valid userID:groupID format",
			input:       "1234:6789",
			wantErr:     false,
			expectedUID: 1234,
			expectedGID: 6789,
		},
		{
			name:        "Valid userID:groupName format",
			input:       "1234:abc",
			wantErr:     false,
			expectedUID: 1234,
			expectedGID: testCurrGID,
		},
		{
			name:        "Valid userID:groupID format",
			input:       "100:200",
			wantErr:     false,
			expectedUID: 100,
			expectedGID: 200,
		},
		{
			name:        "Invalid user format, too many colons",
			input:       "1234:dog:cat",
			wantErr:     true,
			expectedErr: "unable to parse Firelens user",
		},
		{
			name:        "Invalid, non-numeric userID",
			input:       "dog",
			wantErr:     true,
			expectedErr: "unable to determine the user ID",
		},
		{
			name:             "Non-existent directory",
			input:            "1234",
			wantErr:          true,
			invalidDirectory: true,
			expectedErr:      testInvalidDirectoryErr,
		},
	}

	// Store the original os.Chown and restore it after the test
	originalChown := os.Chown
	defer func() {
		osChown = originalChown
	}()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock os.Chown
			var actualUID, actualGID int
			if !tc.invalidDirectory {
				osChown = func(name string, uid, gid int) error {
					actualUID = uid
					actualGID = gid
					return nil
				}

			} else {
				osChown = func(name string, uid, gid int) error {
					return &os.PathError{
						Err: errors.New(testInvalidDirectoryErr),
					}
				}
			}

			err := SetOwnership(testDirectory, tc.input)
			assert.Equal(t, tc.wantErr, err != nil)
			if tc.wantErr {
				// Using ErrorContains since the error chain includes both ParseUser and SetOwnership errors
				assert.ErrorContains(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedUID, actualUID)
				assert.Equal(t, tc.expectedGID, actualGID)
			}
		})
	}
}
