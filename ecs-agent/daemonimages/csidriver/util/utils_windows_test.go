//go:build windows && unit
// +build windows,unit

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

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

package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestParseEndpoint(t *testing.T) {
	testCases := []struct {
		name      string
		endpoint  string
		expScheme string
		expAddr   string
		expErr    error
	}{
		{
			name:      "valid unix endpoint 1",
			endpoint:  "unix:/csi/csi.sock",
			expScheme: "unix",
			expAddr:   `\csi\csi.sock`,
		},
		{
			name:     "invalid endpoint",
			endpoint: "http://127.0.0.1",
			expErr:   fmt.Errorf("unsupported protocol: http"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme, addr, err := ParseEndpoint(tc.endpoint)

			if tc.expErr != nil {
				assert.EqualError(t, err, tc.expErr.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, scheme, tc.expScheme, "scheme mismatches")
				assert.Equal(t, addr, tc.expAddr, "address mismatches")
			}
		})
	}

}

// TestIsBlockDevice tests the IsBlockDevice method for positive and negative cases.
func TestIsBlockDevice(t *testing.T) {
	testCases := []struct {
		name               string
		mockGetVolumeNameF func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error)
		path               string
		expectedResult     bool
		expectedError      error
	}{
		{
			name: "Valid block device",
			mockGetVolumeNameF: func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error) {
				return 1, 0, nil
			},
			path:           "C:\\mount",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Invalid block device",
			mockGetVolumeNameF: func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error) {
				return 0, 0, windows.Errno(1)
			},
			path:           "C:\\invalid\\mount",
			expectedResult: false,
			expectedError:  windows.Errno(1),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldGetVolumeNameF := funcGetVolumeNameForVolumeMountPointW
			defer func() {
				funcGetVolumeNameForVolumeMountPointW = oldGetVolumeNameF
			}()

			// Set the dummy Win32 API method and invoke the IsBlockDevice method.
			funcGetVolumeNameForVolumeMountPointW = tc.mockGetVolumeNameF
			isBlockDevice, err := IsBlockDevice(tc.path)

			assert.Equal(t, tc.expectedResult, isBlockDevice,
				"expected isBlockDevice to be %t, got %t", tc.expectedResult, isBlockDevice)
			assert.ErrorAs(t, err, tc.expectedError,
				"expected error %v, got %v", tc.expectedError, err)
		})
	}
}
