// +build windows

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
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"

	"golang.org/x/sys/windows/registry"

	"github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

var winRegistry dependencies.WindowsRegistry

func init() {
	winRegistry = dependencies.StdRegistry{}
}

func setWinRegistry(mockRegistry dependencies.WindowsRegistry) {
	winRegistry = mockRegistry
}

func getInstallationType(installationType string) (string, error) {
	switch installationType {
	case installationTypeFull:
		return "FULL", nil
	case installationTypeCore:
		return "CORE", nil
	default:
		return "", errors.Errorf("unsupported Installation type:%s", installationType)
	}
}

func getReleaseIdForSACReleases(productName string) (string, error) {
	if strings.HasPrefix(productName, windowsServer2019) {
		return "2019", nil
	} else if strings.HasPrefix(productName, windowsServer2016) {
		return "2016", nil
	}
	err := seelog.Errorf("unsupported productName:%s for Windows SAC Release", productName)
	return "", err
}

func getReleaseIdForLTSCReleases(releaseId string) (string, error) {
	switch releaseId {
	case releaseId2004SAC:
		return releaseId2004SAC, nil
	case releaseId1909SAC:
		return releaseId1909SAC, nil
	default:
		return "", errors.Errorf("unsupported ReleaseId:%s for Windows LTSC Release", releaseId)
	}
}

// GetOperatingSystemFamily() reads the NT current version from windows registry and constructs operating system family string
// In case of any exception this method just returns "windows" as operating system type.
func GetOperatingSystemFamily() string {
	key, err := winRegistry.OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, registry.QUERY_VALUE)
	if err != nil {
		seelog.Errorf("Unable to open Windows registry key to determine Windows version: %v", err)
		return unsupportedWindowsOS
	}
	defer key.Close()

	productName, _, err := key.GetStringValue("ProductName")
	if err != nil {
		seelog.Errorf("Unable to read registry key, ProductName: %v", err)
		return unsupportedWindowsOS
	}
	installationType, _, err := key.GetStringValue("InstallationType")
	if err != nil {
		seelog.Errorf("Unable to read registry key, InstallationType: %v", err)
		return unsupportedWindowsOS
	}
	iType, err := getInstallationType(installationType)
	if err != nil {
		seelog.Errorf("Invalid Installation type found: %v", err)
		return unsupportedWindowsOS
	}

	releaseId := ""
	if strings.HasPrefix(productName, windowsServerDataCenter) {
		releaseIdFromRegistry, _, err := key.GetStringValue("ReleaseId")
		if err != nil {
			seelog.Errorf("Unable to read registry key, ReleaseId: %v", err)
			return unsupportedWindowsOS
		}

		releaseId, err = getReleaseIdForLTSCReleases(releaseIdFromRegistry)
		if err != nil {
			seelog.Errorf("Failed to construct releaseId for Windows LTSC, Error: %v", err)
			return unsupportedWindowsOS
		}
	} else {
		releaseId, err = getReleaseIdForSACReleases(productName)
		if err != nil {
			seelog.Errorf("Failed to construct releaseId for Windows SAC, Error: %v", err)
			return unsupportedWindowsOS
		}
	}
	return fmt.Sprintf(osTypeFormat, releaseId, iType)
}

// parseGMSACapability is used to determine if gMSA support can be enabled
func parseGMSACapability() bool {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), true)
	return checkDomainJoinWithEnvOverride(envStatus)
}

// parseFSxWindowsFileServerCapability is used to determine if fsxWindowsFileServer support can be enabled
func parseFSxWindowsFileServerCapability() bool {
	// fsxwindowsfileserver is not supported on Windows 2016 and non-domain-joined container instances
	status, err := isWindows2016()
	if err != nil || status == true {
		return false
	}

	envStatus := utils.ParseBool(os.Getenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED"), true)
	return checkDomainJoinWithEnvOverride(envStatus)
}

func checkDomainJoinWithEnvOverride(envStatus bool) bool {
	if envStatus {
		// Check if domain join check override is present
		skipDomainJoinCheck := utils.ParseBool(os.Getenv(envSkipDomainJoinCheck), false)
		if skipDomainJoinCheck {
			seelog.Debug("Skipping domain join validation based on environment override")
			return true
		}
		// check if container instance is domain joined.
		// If container instance is not domain joined, explicitly disable feature configuration.
		status, err := isDomainJoined()
		if err == nil && status == true {
			return true
		}
		seelog.Errorf("Unable to determine valid domain join: %v", err)
	}

	return false
}

// isDomainJoined is used to validate if container instance is part of a valid active directory.
// Reference: https://golang.org/src/os/user/lookup_windows.go
func isDomainJoined() (bool, error) {
	var domain *uint16
	var status uint32

	err := syscall.NetGetJoinInformation(nil, &domain, &status)
	if err != nil {
		return false, err
	}

	err = syscall.NetApiBufferFree((*byte)(unsafe.Pointer(domain)))
	if err != nil {
		return false, err
	}

	return status == syscall.NetSetupDomainName, nil
}

// isWindows2016 is used to check if container instance is versioned Windows 2016
// Reference: https://godoc.org/golang.org/x/sys/windows/registry
var isWindows2016 = func() (bool, error) {
	key, err := winRegistry.OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, registry.QUERY_VALUE)

	if err != nil {
		seelog.Errorf("unable to open Windows registry key to determine Windows version: %v", err)
		return false, err
	}
	defer key.Close()

	version, _, err := key.GetStringValue("ProductName")
	if err != nil {
		seelog.Errorf("unable to read current version from Windows registry: %v", err)
		return false, err
	}

	if strings.HasPrefix(version, "Windows Server 2016") {
		return true, nil
	}

	return false, nil
}
