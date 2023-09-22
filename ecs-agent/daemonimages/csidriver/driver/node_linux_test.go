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

package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	volumeID = "voltest"
	nvmeName = "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_voltest"
)

func TestNodeGetVolumeStats(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				VolumePath := "./test"
				err := os.MkdirAll(VolumePath, 0644)
				require.NoError(t, err, "fail to create dir")
				defer os.RemoveAll(VolumePath)

				mockMounter.EXPECT().PathExists(VolumePath).Return(true, nil)

				awsDriver := nodeService{
					mounter: mockMounter,
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   volumeID,
					VolumePath: VolumePath,
				}
				_, err = awsDriver.NodeGetVolumeStats(context.TODO(), req)
				require.NoError(t, err, "fail to get volume stats")
			},
		},
		{
			name: "fail path not exist",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().PathExists(VolumePath).Return(false, nil)

				awsDriver := nodeService{
					mounter: mockMounter,
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   volumeID,
					VolumePath: VolumePath,
				}
				_, err := awsDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErrorCode(t, err, codes.NotFound)
			},
		},
		{
			name: "fail can't determine block device due to no such file",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().PathExists(VolumePath).Return(true, nil)

				awsDriver := nodeService{
					mounter: mockMounter,
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   volumeID,
					VolumePath: VolumePath,
				}
				_, err := awsDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErrorCode(t, err, codes.Internal)
				expectErrorMessage(t, err, fmt.Sprintf("failed to determine whether %s is block device:", VolumePath))
			},
		},
		{
			name: "fail error calling existsPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().PathExists(VolumePath).Return(false, errors.New("get existsPath call fail"))

				awsDriver := nodeService{
					mounter: mockMounter,
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   volumeID,
					VolumePath: VolumePath,
				}
				_, err := awsDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErrorCode(t, err, codes.Internal)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}

}

func expectErrorCode(t *testing.T, actualErr error, expectedCode codes.Code) {
	require.NotNil(t, actualErr, "Expect error but got no error")

	status, ok := status.FromError(actualErr)
	require.True(t, ok, fmt.Sprintf("Failed to get error status from error: %v", actualErr))

	require.Equal(
		t,
		expectedCode,
		status.Code(),
		fmt.Sprintf("Expected error code %d, got %d message %s", codes.InvalidArgument, status.Code(), status.Message()),
	)
}

func expectErrorMessage(t *testing.T, actualErr error, expectedPartialMsg string) {
	require.NotNil(t, actualErr, "Expect error but got no error")

	status, ok := status.FromError(actualErr)
	require.True(t, ok, fmt.Sprintf("Failed to get error status from error: %v", actualErr))

	require.Containsf(
		t,
		status.Message(),
		expectedPartialMsg,
		fmt.Sprintf("Expected partial error message %s", expectedPartialMsg),
	)
}
