//go:build windows
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

	"github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	// The os-family for windows is of the format "WINDOWS_SERVER_<Release>_<FULL or CORE>"
	osFamilyFormat             = "WINDOWS_SERVER_%s_%s"
	windowsServerCore          = "CORE"
	windowsServerFull          = "FULL"
	unsupportedWindowsOSFamily = "windows"

	installationTypeServerCore = "Server Core"
	installationTypeServer     = "Server"

	ecsWinRegistryRootKey  = registry.LOCAL_MACHINE
	ecsWinRegistryRootPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
)

var (
	winRegistry dependencies.WindowsRegistry
	// osReleaseFromBuildNumber is the mapping from build number to the Windows release.
	// Reference- https://docs.microsoft.com/en-us/windows-server/get-started/windows-server-release-info
	osReleaseFromBuildNumber = map[int]string{
		14393: "2016",
		17763: "2019",
		19041: "2004",
		19042: "20H2",
		20348: "2022",
	}
	// windowsGetVersionFunc is the method used to obtain information about current Windows version.
	windowsGetVersionFunc = windows.RtlGetVersion
)

func init() {
	winRegistry = dependencies.StdRegistry{}
}

// getInstallationType returns the installation type as either CORE or FULL.
func getInstallationType(installationType string) (string, error) {
	switch installationType {
	case installationTypeServer:
		return windowsServerFull, nil
	case installationTypeServerCore:
		return windowsServerCore, nil
	default:
		return "", errors.Errorf("unsupported installation type: %s", installationType)
	}
}

// GetOSFamily returns the operating system family string.
// On Windows, it would be of the format "WINDOWS_SERVER_<Release>_<FULL or CORE>"
// In case of any exception this method just returns "windows" as operating system family.
func GetOSFamily() string {
	// Find the build number of the os on which agent is running.
	versionInfo := windowsGetVersionFunc()
	buildNumber := int(versionInfo.BuildNumber)
	osRelease, ok := osReleaseFromBuildNumber[buildNumber]
	if !ok {
		seelog.Errorf("windows release with build number [%d] is unsupported", buildNumber)
		return unsupportedWindowsOSFamily
	}

	// Find the installation type from the Windows registry.
	key, err := winRegistry.OpenKey(ecsWinRegistryRootKey, ecsWinRegistryRootPath, registry.QUERY_VALUE)
	if err != nil {
		seelog.Errorf("unable to open Windows registry key to determine Windows installation type: %v", err)
		return unsupportedWindowsOSFamily
	}
	defer key.Close()

	installationType, _, err := key.GetStringValue("InstallationType")
	if err != nil {
		seelog.Errorf("unable to read registry key, InstallationType: %v", err)
		return unsupportedWindowsOSFamily
	}
	iType, err := getInstallationType(installationType)
	if err != nil {
		seelog.Errorf("invalid Installation type found: %v", err)
		return unsupportedWindowsOSFamily
	}

	// Construct the OSFamily attribute from the OS release and installation type.
	osFamily := fmt.Sprintf(osFamilyFormat, osRelease, iType)
	seelog.Debugf("operating system family is: %s", osFamily)

	return osFamily
}
