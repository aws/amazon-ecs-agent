//go:build linux && unit
// +build linux,unit

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

package driver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/driver/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	volumeID        = "voltest"
	nvmeName        = "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_voltest"
	symlinkFileInfo = fs.FileInfo(&fakeFileInfo{nvmeName, os.ModeSymlink})
)

func TestNodeStageVolume(t *testing.T) {
	var (
		targetPath     = "/test/path"
		devicePath     = "/dev/fake"
		nvmeDevicePath = "/dev/nvmefake1n1"
		deviceFileInfo = fs.FileInfo(&fakeFileInfo{devicePath, os.ModeDevice})
		stdVolCap      = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: FSTypeXfs,
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		}
		// With few exceptions, all "success" non-block cases have roughly the same
		// expected calls and only care about testing the FormatAndMountSensitiveWithFormatOptions call. The
		// exceptions should not call this, instead they should define expectMock
		// from scratch.
		successExpectMock = func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
			mockMounter.EXPECT().PathExists(gomock.Eq(targetPath)).Return(false, nil)
			mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
			mockMounter.EXPECT().GetDeviceNameFromMount(targetPath).Return("", 1, nil)
			mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(true, nil)
			mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(devicePath)).Return(deviceFileInfo, nil)
			mockMounter.EXPECT().NeedResize(gomock.Eq(devicePath), gomock.Eq(targetPath)).Return(false, nil)
		}
	)
	testCases := []struct {
		name         string
		request      *csi.NodeStageVolumeRequest
		inFlightFunc func(*internal.InFlight) *internal.InFlight
		expectMock   func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier)
		expectedCode codes.Code
	}{
		{
			name: "success normal",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeId:          volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				successExpectMock(mockMounter, mockDeviceIdentifier)
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Any(), gomock.Nil(), gomock.Len(0))
			},
		},
		{
			name: "success normal [raw block]",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Len(0)).Times(0)
			},
		},
		{
			name: "success fsType ext4",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: FSTypeExt4,
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				successExpectMock(mockMounter, mockDeviceIdentifier)
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeExt4), gomock.Any(), gomock.Nil(), gomock.Len(0))
			},
		},

		{
			name: "success fsType ext3",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: FSTypeExt3,
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				successExpectMock(mockMounter, mockDeviceIdentifier)
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeExt3), gomock.Any(), gomock.Nil(), gomock.Len(0))
			},
		},
		{
			name: "success mount with default fsType xfs",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "",
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				successExpectMock(mockMounter, mockDeviceIdentifier)
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeXfs), gomock.Any(), gomock.Nil(), gomock.Len(0))
			},
		},
		{
			name: "success device already mounted at target",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeId:          volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockMounter.EXPECT().PathExists(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().GetDeviceNameFromMount(targetPath).Return(devicePath, 1, nil)
				mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(true, nil)
				mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(devicePath)).Return(deviceFileInfo, nil)

				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Len(0)).Times(0)
			},
		},
		{
			name: "success nvme device already mounted at target",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeId:          volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockMounter.EXPECT().PathExists(gomock.Eq(targetPath)).Return(true, nil)

				// If the device is nvme GetDeviceNameFromMount should return the
				// canonical device path
				mockMounter.EXPECT().GetDeviceNameFromMount(targetPath).Return(nvmeDevicePath, 1, nil)

				// The publish context device path may not exist but the driver should
				// find the canonical device path (see TestFindDevicePath), compare it
				// to the one returned by GetDeviceNameFromMount, and then skip
				// FormatAndMountSensitiveWithFormatOptions
				mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(false, nil)
				mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(nvmeName)).Return(symlinkFileInfo, nil)
				mockDeviceIdentifier.EXPECT().EvalSymlinks(gomock.Eq(symlinkFileInfo.Name())).Return(nvmeDevicePath, nil)

				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Len(0)).Times(0)
			},
		},
		{
			name: "fail no VolumeId",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail no mount",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail no StagingTargetPath",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:   map[string]string{DevicePathKey: devicePath},
				VolumeCapability: stdVolCap,
				VolumeId:         volumeID,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail no VolumeCapability",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeId:          volumeID,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail invalid VolumeCapability",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_UNKNOWN,
					},
				},
				VolumeId: volumeID,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail no devicePath",
			request: &csi.NodeStageVolumeRequest{
				VolumeCapability: stdVolCap,
				VolumeId:         volumeID,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail invalid volumeContext",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeContext:     map[string]string{VolumeAttributePartition: "partition1"},
				VolumeId:          volumeID,
			},
			expectedCode: codes.InvalidArgument,
		},
		{
			name: "fail with in-flight request",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeId:          volumeID,
			},
			inFlightFunc: func(inFlight *internal.InFlight) *internal.InFlight {
				inFlight.Insert(volumeID)
				return inFlight
			},
			expectedCode: codes.Aborted,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			mockMounter := NewMockMounter(mockCtl)
			mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

			inFlight := internal.NewInFlight()
			if tc.inFlightFunc != nil {
				tc.inFlightFunc(inFlight)
			}

			awsDriver := &nodeService{
				mounter:          mockMounter,
				deviceIdentifier: mockDeviceIdentifier,
				inFlight:         inFlight,
			}

			if tc.expectMock != nil {
				tc.expectMock(*mockMounter, *mockDeviceIdentifier)
			}

			_, err := awsDriver.NodeStageVolume(context.TODO(), tc.request)
			if tc.expectedCode != codes.OK {
				expectErr(t, err, tc.expectedCode)
			} else if err != nil {
				t.Fatalf("Expect no error but got: %v", err)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	targetPath := "/test/path"
	devicePath := "/dev/fake"

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
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceNameFromMount(gomock.Eq(targetPath)).Return(devicePath, 1, nil)
				mockMounter.EXPECT().Unstage(gomock.Eq(targetPath)).Return(nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          volumeID,
				}

				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				if err != nil {
					t.Fatalf("Expect no error but got: %v", err)
				}
			},
		},
		{
			name: "success no device mounted at target",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceNameFromMount(gomock.Eq(targetPath)).Return(devicePath, 0, nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          volumeID,
				}
				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				if err != nil {
					t.Fatalf("Expect no error but got: %v", err)
				}
			},
		},
		{
			name: "success device mounted at multiple targets",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceNameFromMount(gomock.Eq(targetPath)).Return(devicePath, 2, nil)
				mockMounter.EXPECT().Unstage(gomock.Eq(targetPath)).Return(nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          volumeID,
				}

				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				if err != nil {
					t.Fatalf("Expect no error but got: %v", err)
				}
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
				}

				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no StagingTargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				req := &csi.NodeUnstageVolumeRequest{
					VolumeId: volumeID,
				}
				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail GetDeviceName returns error",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceNameFromMount(gomock.Eq(targetPath)).Return("", 0, errors.New("GetDeviceName faield"))

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          volumeID,
				}

				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.Internal)
			},
		},
		{
			name: "fail another operation in-flight on given volumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMounter := NewMockMounter(mockCtl)
				mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

				awsDriver := &nodeService{
					mounter:          mockMounter,
					deviceIdentifier: mockDeviceIdentifier,
					inFlight:         internal.NewInFlight(),
				}

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          volumeID,
				}

				awsDriver.inFlight.Insert(volumeID)
				_, err := awsDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.Aborted)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

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

func TestFindDevicePath(t *testing.T) {
	devicePath := "/dev/xvdaa"
	nvmeDevicePath := "/dev/nvme1n1"
	volumeID := "vol-test"
	nvmeName := "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_voltest"
	deviceFileInfo := fs.FileInfo(&fakeFileInfo{devicePath, os.ModeDevice})
	symlinkFileInfo := fs.FileInfo(&fakeFileInfo{nvmeName, os.ModeSymlink})
	nvmeDevicePathSymlinkFileInfo := fs.FileInfo(&fakeFileInfo{nvmeDevicePath, os.ModeSymlink})
	type testCase struct {
		name             string
		devicePath       string
		volumeID         string
		partition        string
		expectMock       func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier)
		expectDevicePath string
		expectError      string
	}
	testCases := []testCase{
		{
			name:       "11: device path exists and nvme device path exists",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				gomock.InOrder(
					mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(true, nil),
					mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(devicePath)).Return(nvmeDevicePathSymlinkFileInfo, nil),
					mockDeviceIdentifier.EXPECT().EvalSymlinks(gomock.Eq(devicePath)).Return(nvmeDevicePath, nil),
				)
			},
			expectDevicePath: nvmeDevicePath,
		},
		{
			name:       "10: device path exists and nvme device path doesn't exist",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				gomock.InOrder(
					mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(true, nil),
					mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(devicePath)).Return(deviceFileInfo, nil),
				)
			},
			expectDevicePath: devicePath,
		},
		{
			name:       "01: device path doesn't exist and nvme device path exists",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				gomock.InOrder(
					mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(false, nil),

					mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(nvmeName)).Return(symlinkFileInfo, nil),
					mockDeviceIdentifier.EXPECT().EvalSymlinks(gomock.Eq(symlinkFileInfo.Name())).Return(nvmeDevicePath, nil),
				)
			},
			expectDevicePath: nvmeDevicePath,
		},
		{
			name:       "00: device path doesn't exist and nvme device path doesn't exist",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				gomock.InOrder(
					mockMounter.EXPECT().PathExists(gomock.Eq(devicePath)).Return(false, nil),

					mockDeviceIdentifier.EXPECT().Lstat(gomock.Eq(nvmeName)).Return(nil, os.ErrNotExist),
				)
			},
			expectError: errNoDevicePathFound(devicePath, volumeID).Error(),
		},
	}
	// The partition variant of each case should be the same except the partition
	// is expected to be appended to devicePath
	generatedTestCases := []testCase{}
	for _, tc := range testCases {
		tc.name += " (with partition)"
		tc.partition = "1"
		if tc.expectDevicePath == devicePath {
			tc.expectDevicePath += tc.partition
		} else if tc.expectDevicePath == nvmeDevicePath {
			tc.expectDevicePath += "p" + tc.partition
		}
		generatedTestCases = append(generatedTestCases, tc)
	}
	testCases = append(testCases, generatedTestCases...)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			mockMounter := NewMockMounter(mockCtl)
			mockDeviceIdentifier := NewMockDeviceIdentifier(mockCtl)

			nodeDriver := nodeService{
				mounter:          mockMounter,
				deviceIdentifier: mockDeviceIdentifier,
				inFlight:         internal.NewInFlight(),
			}

			if tc.expectMock != nil {
				tc.expectMock(*mockMounter, *mockDeviceIdentifier)
			}

			devicePath, err := nodeDriver.findDevicePath(tc.devicePath, tc.volumeID, tc.partition)
			if tc.expectError != "" {
				assert.EqualError(t, err, tc.expectError)
			} else {
				assert.Equal(t, tc.expectDevicePath, devicePath)
				assert.NoError(t, err)
			}
		})
	}
}

type fakeFileInfo struct {
	name string
	mode os.FileMode
}

func (fi *fakeFileInfo) Name() string {
	return fi.name
}

func (fi *fakeFileInfo) Size() int64 {
	return 0
}

func (fi *fakeFileInfo) Mode() os.FileMode {
	return fi.mode
}

func (fi *fakeFileInfo) ModTime() time.Time {
	return time.Now()
}

func (fi *fakeFileInfo) IsDir() bool {
	return false
}

func (fi *fakeFileInfo) Sys() interface{} {
	return nil
}

func expectErr(t *testing.T, actualErr error, expectedCode codes.Code) {
	if actualErr == nil {
		t.Fatalf("Expect error but got no error")
	}

	status, ok := status.FromError(actualErr)
	if !ok {
		t.Fatalf("Failed to get error status code from error: %v", actualErr)
	}

	if status.Code() != expectedCode {
		t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, status.Code(), status.Message())
	}
}
