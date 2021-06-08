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
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

const (
	// envSkipDomainJoinCheck is an environment setting that can be used to skip
	// domain join check validation. This is useful for integration and
	// functional-tests but should not be set for any non-test use-case.
	envSkipDomainJoinCheck = "ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION"
)

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
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		seelog.Errorf("Unable to open Windows registry key to determine Windows version: %v", err)
		return false, err
	}
	defer key.Close()

	version, _, err := key.GetStringValue("ProductName")
	if err != nil {
		seelog.Errorf("Unable to read current version from Windows registry: %v", err)
		return false, err
	}

	if strings.HasPrefix(version, "Windows Server 2016") {
		return true, nil
	}

	return false, nil
}
