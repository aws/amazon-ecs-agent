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

package config

import (
	"testing"

	mock_dependencies "github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func fakeWindowsGetVersionFunc(buildNumber int) func() *windows.OsVersionInfoEx {
	return func() *windows.OsVersionInfoEx {
		return &windows.OsVersionInfoEx{BuildNumber: uint32(buildNumber)}
	}
}

func getMockWindowsRegistryKey(ctrl *gomock.Controller) *mock_dependencies.MockRegistryKey {
	mockWinRegistry := mock_dependencies.NewMockWindowsRegistry(ctrl)
	mockKey := mock_dependencies.NewMockRegistryKey(ctrl)

	winRegistry = mockWinRegistry
	mockWinRegistry.EXPECT().OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, gomock.Any()).Return(mockKey, nil)

	return mockKey
}

func TestGetOSFamilyForWS2022Core(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(20348)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2022_CORE", GetOSFamily())
}

func TestGetOSFamilyForWS2022Full(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(20348)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2022_FULL", GetOSFamily())
}

func TestGetOSFamilyForWS2019Core(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(17763)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2019_CORE", GetOSFamily())
}

func TestGetOSFamilyForWS2019Full(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(17763)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2019_FULL", GetOSFamily())
}

func TestGetOSFamilyForWS2016Full(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(14393)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2016_FULL", GetOSFamily())
}

func TestGetOSFamilyForWS2004Core(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(19041)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_2004_CORE", GetOSFamily())
}

func TestGetOSFamilyForWS20H2Core(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(19042)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, "WINDOWS_SERVER_20H2_CORE", GetOSFamily())
}

func TestGetOSFamilyForInvalidBuildNumber(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(12345)
	assert.Equal(t, unsupportedWindowsOSFamily, GetOSFamily())
}

func TestGetOSFamilyForKeyError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(19042)
	mockWinRegistry := mock_dependencies.NewMockWindowsRegistry(ctrl)
	mockKey := mock_dependencies.NewMockRegistryKey(ctrl)
	winRegistry = mockWinRegistry

	gomock.InOrder(
		mockWinRegistry.EXPECT().OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, gomock.Any()).Return(mockKey, registry.ErrNotExist),
	)

	assert.Equal(t, unsupportedWindowsOSFamily, GetOSFamily())
}

func TestGetOSFamilyForInstallationTypeNotExistError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(19042)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), registry.ErrNotExist),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, unsupportedWindowsOSFamily, GetOSFamily())
}

func TestGetOSFamilyForInvalidInstallationType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	windowsGetVersionFunc = fakeWindowsGetVersionFunc(19042)
	mockKey := getMockWindowsRegistryKey(ctrl)
	gomock.InOrder(
		mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core Invalid`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)

	assert.Equal(t, unsupportedWindowsOSFamily, GetOSFamily())
}
