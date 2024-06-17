//go:build windows
// +build windows

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
	os "os"
	"testing"

	diskapi "github.com/kubernetes-csi/csi-proxy/client/api/disk/v1"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/driver/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	volumeID = "VolTest"
)

func TestNodeStageVolume(t *testing.T) {

	var (
		targetPath = "C:/var/lib/kubelet/csi-driver/mount"
		devicePath = "/dev/fake"
		diskNumber = "0"
		stdVolCap  = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: FSTypeNtfs,
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
			mockDeviceIdentifier.EXPECT().ListDiskIDs().AnyTimes().Return(map[uint32]*diskapi.DiskIDs{0: {Page83: "", SerialNumber: "VolTest"}}, nil)
			mockMounter.EXPECT().PathExists(gomock.Eq(targetPath)).Return(false, nil)
			mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
			mockMounter.EXPECT().GetDeviceNameFromMount(targetPath).Return("", 1, nil)
			mockMounter.EXPECT().NeedResize(gomock.Eq(diskNumber), gomock.Eq(targetPath)).Return(false, nil)
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
				mockMounter.EXPECT().FormatAndMountSensitiveWithFormatOptions(gomock.Eq(diskNumber), gomock.Eq(targetPath), gomock.Eq(FSTypeNtfs), gomock.Any(), gomock.Nil(), gomock.Len(0))
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
			name: "success device already mounted at target",
			request: &csi.NodeStageVolumeRequest{
				PublishContext:    map[string]string{DevicePathKey: devicePath},
				StagingTargetPath: targetPath,
				VolumeCapability:  stdVolCap,
				VolumeId:          volumeID,
			},
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockDeviceIdentifier.EXPECT().ListDiskIDs().AnyTimes().Return(map[uint32]*diskapi.DiskIDs{0: {Page83: "", SerialNumber: "VolTest"}}, nil)
				mockMounter.EXPECT().PathExists(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().GetDeviceNameFromMount(targetPath).Return(diskNumber, 1, nil)

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
	targetPath := "C:/var/lib/kubelet/csi-driver/mount"
	devicePath := "xvde"
	diskNumber := "2"

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

				mockMounter.EXPECT().GetDeviceNameFromMount(gomock.Eq(targetPath)).Return(diskNumber, 1, nil)
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

func TestFindDevicePath(t *testing.T) {
	devicePath := "xvde"
	diskNumber := "0"
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
			name:       "device path exists",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockDeviceIdentifier.EXPECT().ListDiskIDs().AnyTimes().Return(map[uint32]*diskapi.DiskIDs{0: {Page83: "", SerialNumber: "VolTest"}}, nil)
			},
			expectDevicePath: diskNumber,
		},
		{
			name:       "device path doesn't exist",
			devicePath: devicePath,
			volumeID:   volumeID,
			partition:  "",
			expectMock: func(mockMounter MockMounter, mockDeviceIdentifier MockDeviceIdentifier) {
				mockDeviceIdentifier.EXPECT().ListDiskIDs().AnyTimes().Return(map[uint32]*diskapi.DiskIDs{0: {Page83: "", SerialNumber: ""}}, nil)
			},
			expectError: fmt.Errorf("disk number for device path %q volume id %q not found", devicePath, volumeID).Error(),
		},
	}

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
