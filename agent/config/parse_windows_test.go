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
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows/registry"

	"os"
	"testing"

	"github.com/golang/mock/gomock"

	mock_dependencies "github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies/mocks"
)

func TestParseGMSACapability(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "False")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	assert.False(t, parseGMSACapability())
}

func TestParseBooleanEnvVar(t *testing.T) {
	os.Setenv("EXAMPLE_SETTING", "True")
	defer os.Unsetenv("EXAMPLE_SETTING")

	assert.True(t, parseBooleanDefaultFalseConfig("EXAMPLE_SETTING").Enabled())
	assert.True(t, parseBooleanDefaultTrueConfig("EXAMPLE_SETTING").Enabled())

	os.Setenv("EXAMPLE_SETTING", "False")
	assert.False(t, parseBooleanDefaultFalseConfig("EXAMPLE_SETTING").Enabled())
	assert.False(t, parseBooleanDefaultTrueConfig("EXAMPLE_SETTING").Enabled())
}

func TestParseFSxWindowsFileServerCapability(t *testing.T) {
	isWindows2016 = func() (bool, error) {
		return false, nil
	}
	os.Setenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED", "False")
	defer os.Unsetenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED")

	assert.False(t, parseFSxWindowsFileServerCapability())
}

func getMockRegistry(controller *gomock.Controller) *mock_dependencies.MockWindowsRegistry {
	winRegistry := mock_dependencies.NewMockWindowsRegistry(controller)
	setWinRegistry(winRegistry)
	return winRegistry
}

func getMockKey(t *testing.T) *mock_dependencies.MockRegistryKey {
	ctrl := gomock.NewController(t)
	winRegistry := getMockRegistry(ctrl)
	mockKey := mock_dependencies.NewMockRegistryKey(ctrl)
	winRegistry.EXPECT().OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, gomock.Any()).Return(mockKey, nil)
	return mockKey
}

func TestGetOperatingSystemFamilyForWS2019Core(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server 2019 Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`1809`, uint32(0), nil)
	mockKey.EXPECT().Close()
	assert.Equal(t, "WINDOWS_SERVER_2019_CORE", GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForWS2019Full(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server 2019 Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`1809`, uint32(0), nil)
	mockKey.EXPECT().Close()

	assert.Equal(t, "WINDOWS_SERVER_2019_FULL", GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForWS2016Full(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server 2016 Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`1607`, uint32(0), nil)
	mockKey.EXPECT().Close()

	assert.Equal(t, "WINDOWS_SERVER_2016_FULL", GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForWS2004Core(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`2004`, uint32(0), nil)
	mockKey.EXPECT().Close()

	assert.Equal(t, "WINDOWS_SERVER_2004_CORE", GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForKeyError(t *testing.T) {
	mockKey := getMockKey(t)
	winRegistry := getMockRegistry(gomock.NewController(t))
	winRegistry.EXPECT().OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, gomock.Any()).Return(mockKey, registry.ErrNotExist)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForProductNameNotExistError(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return("", uint32(0), registry.ErrNotExist)

	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForInstallationTypeNotExistError(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), registry.ErrNotExist)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForInvalidInstallationType(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core Invalid`, uint32(0), nil)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForReleaseIdNotExistError(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`2004`, uint32(0), registry.ErrNotExist)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForInvalidLTSCReleaseId(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server Datacenter`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("ReleaseId").Return(`2028`, uint32(0), registry.ErrNotExist)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}

func TestGetOperatingSystemFamilyForInvalidSACReleaseId(t *testing.T) {
	mockKey := getMockKey(t)
	mockKey.EXPECT().GetStringValue("ProductName").Return(`Windows Server BadVersion`, uint32(0), nil)
	mockKey.EXPECT().GetStringValue("InstallationType").Return(`Server Core`, uint32(0), nil)
	mockKey.EXPECT().Close()
	assert.Equal(t, unsupportedWindowsOS, GetOperatingSystemFamily())
}
