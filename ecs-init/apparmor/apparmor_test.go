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

package apparmor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/pkg/aaparser"
	aaprofile "github.com/docker/docker/profiles/apparmor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultProfile(t *testing.T) {
	testCases := []struct {
		name             string
		profileName      string
		isLoadedResponse bool
		isLoadedError    error
		loadError        error
		statErrors       map[string]error
		profileContent   []string
		expectedError    error
	}{
		{
			name:             "ProfileIsAlreadyLoaded",
			profileName:      "testProfile.txt",
			isLoadedResponse: true,
			isLoadedError:    nil,
			loadError:        nil,
			profileContent:   []string{"#include <tunables/global>", "#include <abstractions/base>"},
			expectedError:    nil,
		},
		{
			name:             "ProfileNotLoaded",
			profileName:      "testProfile.txt",
			isLoadedResponse: false,
			isLoadedError:    nil,
			loadError:        nil,
			profileContent:   []string{"#include <tunables/global>", "#include <abstractions/base>"},
			expectedError:    nil,
		},
		{
			name:             "IsLoadedError",
			profileName:      "testProfile.txt",
			isLoadedResponse: false,
			isLoadedError:    errors.New("mock isLoaded error"),
			loadError:        nil,
			expectedError:    errors.New("mock isLoaded error"),
		},
		{
			name:             "LoadProfileError",
			profileName:      "testProfile.txt",
			isLoadedResponse: false,
			isLoadedError:    nil,
			loadError:        errors.New("mock load error"),
			expectedError:    errors.New("mock load error"),
		},
		{
			name:        "MissingTunablesGlobal",
			profileName: "testProfile.txt",
			statErrors: map[string]error{
				"tunables/global": os.ErrNotExist,
			},
			profileContent: []string{"@{PROC}=/proc/", "#include <abstractions/base>"},
		},
		{
			name:        "MissingAbstractionsBase",
			profileName: "testProfile.txt",
			statErrors: map[string]error{
				"abstractions/base": os.ErrNotExist,
			},
			profileContent: []string{"#include <tunables/global>"},
		},
		{
			name:        "MissingIncludes",
			profileName: "testProfile.txt",
			statErrors: map[string]error{
				"tunables/global":   os.ErrNotExist,
				"abstractions/base": os.ErrNotExist,
			},
			profileContent: []string{"@{PROC}=/proc/"},
		},
		{
			name:        "StatError",
			profileName: "testProfile.txt",
			statErrors: map[string]error{
				"tunables/global": os.ErrPermission,
			},
			expectedError: os.ErrPermission,
		},
	}
	defer func() {
		isProfileLoaded = aaprofile.IsLoaded
		loadPath = aaparser.LoadProfile
		createFile = os.Create
		statFile = os.Stat
	}()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir := os.TempDir()
			filePath, err := os.MkdirTemp(tmpdir, "test")
			require.NoError(t, err)

			var profilePath string
			createFile = func(profileName string) (*os.File, error) {
				f, err := os.Create(filepath.Join(filePath, tc.profileName))
				profilePath = f.Name()
				return f, err
			}

			statFile = func(fileName string) (os.FileInfo, error) {
				relativePath, err := filepath.Rel(appArmorProfileDir, fileName)
				require.NoError(t, err)
				return nil, tc.statErrors[relativePath]
			}

			defer os.RemoveAll(filePath)
			isProfileLoaded = func(profileName string) (bool, error) {
				return tc.isLoadedResponse, tc.isLoadedError
			}

			loadPath = func(profile string) error {
				return tc.loadError
			}

			err = LoadDefaultProfile(tc.profileName)
			if tc.loadError == nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Error(t, err)
			}

			if err == nil {
				b, err := os.ReadFile(profilePath)
				require.NoError(t, err)
				profile := string(b)

				for _, s := range tc.profileContent {
					assert.Contains(t, profile, s)
				}
			}
		})
	}
}
