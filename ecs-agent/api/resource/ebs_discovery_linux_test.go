//go:build linux && unit
// +build linux,unit

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
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLsblkOutput(t *testing.T) {
	mkError := func(deviceName string, volumeId string) string {
		return fmt.Sprintf("cannot find EBS volume with device name: %v and volume ID: %v",
			deviceName, volumeId)
	}

	tcs := []struct {
		name                     string
		lsblkOutput              LsblkOutput
		deviceName               string
		volumeId                 string
		hasXenSupport            bool
		expectedActualDeviceName string
		expectedError            string
	}{
		{
			name: "nitro device found with the same name",
			lsblkOutput: LsblkOutput{[]BlockDevice{
				{Name: "mydevice", Serial: "myvolumeid"},
			}},
			deviceName:               "/dev/mydevice",
			volumeId:                 "myvolumeid",
			expectedActualDeviceName: "mydevice",
		},
		{
			name: "nitro device found with another name",
			lsblkOutput: LsblkOutput{[]BlockDevice{
				{Name: "mydevice", Serial: "myvolumeid"},
			}},
			deviceName:               "/dev/otherdevice",
			volumeId:                 "myvolumeid",
			expectedActualDeviceName: "mydevice",
		},
		{
			name: "nitro device found with another name multiple devices",
			lsblkOutput: LsblkOutput{[]BlockDevice{
				{Name: "mydevice2", Serial: "myvolumeid2"},
				{Name: "mydevice", Serial: "myvolumeid"},
			}},
			deviceName:               "/dev/otherdevice",
			volumeId:                 "myvolumeid",
			expectedActualDeviceName: "mydevice",
		},
		{
			name: "nitro device not found",
			lsblkOutput: LsblkOutput{[]BlockDevice{
				{Name: "mydevice", Serial: "myvolumeid"},
			}},
			deviceName:    "/dev/mydevice",
			volumeId:      "othervolumeid",
			expectedError: mkError("mydevice", "othervolumeid"),
		},
		{
			name:                     "xen received sd found sd",
			lsblkOutput:              LsblkOutput{[]BlockDevice{{Name: "sddevice"}}},
			deviceName:               "/dev/sddevice",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "sddevice",
		},
		{
			name:                     "xen received xvd found xvd",
			lsblkOutput:              LsblkOutput{[]BlockDevice{{Name: "xvddevice"}}},
			deviceName:               "/dev/xvddevice",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "xvddevice",
		},
		{
			name:                     "xen received sd found xvd",
			lsblkOutput:              LsblkOutput{[]BlockDevice{{Name: "xvddevice"}}},
			deviceName:               "/dev/sddevice",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "xvddevice",
		},
		{
			name:                     "xen received xvd found sd",
			lsblkOutput:              LsblkOutput{[]BlockDevice{{Name: "sddevice"}}},
			deviceName:               "/dev/xvddevice",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "sddevice",
		},
		{
			name:          "xen device not found",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "sda"}}},
			deviceName:    "/dev/xvdb",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("xvdb", "volumeid"),
		},
		{
			name:          "xen received sda shouldn't match with found a",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "a"}}},
			deviceName:    "/dev/sda",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("sda", "volumeid"),
		},
		{
			name:          "xen received xvda shouldn't match with found a",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "a"}}},
			deviceName:    "/dev/xvda",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("xvda", "volumeid"),
		},
		{
			name:          "xen received a shouldn't match with found sda",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "sda"}}},
			deviceName:    "/dev/a",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("a", "volumeid"),
		},
		{
			name:          "xen received a shouldn't match with found xvda",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "xvda"}}},
			deviceName:    "/dev/a",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("a", "volumeid"),
		},
		{
			name:                     "xen received abc should match with found abc",
			lsblkOutput:              LsblkOutput{[]BlockDevice{{Name: "abc"}}},
			deviceName:               "/dev/abc",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "abc",
		},
		{
			name:          "xen received abc shouldn't match with found xyz",
			lsblkOutput:   LsblkOutput{[]BlockDevice{{Name: "xyz"}}},
			deviceName:    "/dev/abc",
			volumeId:      "volumeid",
			hasXenSupport: true,
			expectedError: mkError("abc", "volumeid"),
		},
		{
			name: "xen received xvd found sd multiple devices",
			lsblkOutput: LsblkOutput{[]BlockDevice{
				{Name: "some_other_device"},
				{Name: "sddevice"},
			}},
			deviceName:               "/dev/xvddevice",
			volumeId:                 "volumeid",
			hasXenSupport:            true,
			expectedActualDeviceName: "sddevice",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			actualDeviceName, err := parseLsblkOutput(
				&tc.lsblkOutput, tc.deviceName, tc.volumeId, tc.hasXenSupport)
			if tc.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedActualDeviceName, actualDeviceName)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func TestWithXenSupport(t *testing.T) {
	t.Run("no xen support by default", func(t *testing.T) {
		assert.False(t, NewDiscoveryClient(context.Background()).HasXenSupport())
	})
	t.Run("with xen support", func(t *testing.T) {
		assert.True(t, NewDiscoveryClient(context.Background(), WithXenSupport()).HasXenSupport())
	})
}
