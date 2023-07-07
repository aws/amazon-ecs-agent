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
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/cihub/seelog"
	"golang.org/x/sys/windows/registry"
)

const (
	// envSkipWindowsServerVersionCheck is an environment setting that can be used
	// to skip the windows server version check. This is useful for testing and
	// should not be set for any non-test use-case.
	envSkipWindowsServerVersionCheck = "ZZZ_SKIP_WINDOWS_SERVER_VERSION_CHECK_NOT_SUPPORTED_IN_PRODUCTION"
	gmsaPluginGUID                   = "{859E1386-BDB4-49E8-85C7-3070B13920E1}"
)

var (
	fnQueryDomainlessGmsaPluginSubKeys = queryDomainlessGmsaPluginSubKeys
)

// parseGMSACapability is used to determine if gMSA support can be enabled
func parseGMSACapability() BooleanDefaultFalse {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), true)
	return checkDomainJoinWithEnvOverride(envStatus)
}

// parseGMSADomainlessCapability is used to determine if gMSA domainless support can be enabled
func parseGMSADomainlessCapability() BooleanDefaultFalse {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), false)
	if envStatus {
		// gmsaDomainless is not supported on Windows 2016
		isWindows2016, err := IsWindows2016()
		if err != nil || isWindows2016 {
			return BooleanDefaultFalse{Value: ExplicitlyDisabled}
		}

		// gmsaDomainless is not supported if the plugin is not installed on the instance
		installed, err := isDomainlessGmsaPluginInstalled()
		if err != nil || !installed {
			return BooleanDefaultFalse{Value: ExplicitlyDisabled}
		}
		return BooleanDefaultFalse{Value: ExplicitlyEnabled}
	}
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
}

// parseFSxWindowsFileServerCapability is used to determine if fsxWindowsFileServer support can be enabled
func parseFSxWindowsFileServerCapability() BooleanDefaultTrue {
	// fsxwindowsfileserver is not supported on Windows 2016.
	status, err := IsWindows2016()
	if err != nil || status == true {
		return BooleanDefaultTrue{Value: ExplicitlyDisabled}
	}

	// By default, or if ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED is set as true, agent will
	// broadcast the FSx capability. Only when ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED is
	// explicitly set as false, the instance will not broadcast FSx capability.
	return parseBooleanDefaultTrueConfig("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED")
}

func checkDomainJoinWithEnvOverride(envStatus bool) BooleanDefaultFalse {
	if envStatus {
		// Check if domain join check override is present
		skipDomainJoinCheck := utils.ParseBool(os.Getenv(envSkipDomainJoinCheck), false)
		if skipDomainJoinCheck {
			seelog.Debug("Skipping domain join validation based on environment override")
			return BooleanDefaultFalse{Value: ExplicitlyEnabled}
		}
		// check if container instance is domain joined.
		// If container instance is not domain joined, explicitly disable feature configuration.
		status, err := isDomainJoined()
		if err != nil {
			seelog.Errorf("Unable to determine valid domain join with err: %v", err)
		}
		if status == true {
			return BooleanDefaultFalse{Value: ExplicitlyEnabled}
		}
	}
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
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

// Making it visible for unit testing
var execCommand = exec.Command

var IsWindows2016 = func() (bool, error) {
	// Check for environment override before proceeding.
	envSkipWindowsServerVersionCheck := utils.ParseBool(os.Getenv(envSkipWindowsServerVersionCheck), false)
	if envSkipWindowsServerVersionCheck {
		seelog.Debug("Skipping windows server version check based on environment override")
		return false, nil
	}

	cmd := "systeminfo | findstr /B /C:\"OS Name\""
	out, err := execCommand("powershell", "-Command", cmd).CombinedOutput()
	if err != nil {
		return false, err
	}

	str := string(out)
	isWS2016 := strings.Contains(str, "Microsoft Windows Server 2016 Datacenter")

	return isWS2016, nil
}

var queryDomainlessGmsaPluginSubKeys = func() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\CCG\COMClasses\`, registry.READ)
	if err != nil {
		seelog.Errorf("Failed to open registry key SYSTEM\\CurrentControlSet\\Control\\CCG\\COMClasses with error: %v", err)
		return nil, err
	}
	defer k.Close()
	stat, err := k.Stat()
	if err != nil {
		seelog.Errorf("Failed to stat registry key SYSTEM\\CurrentControlSet\\Control\\CCG\\COMClasses with error: %v", err)
		return nil, err
	}
	subKeys, err := k.ReadSubKeyNames(int(stat.SubKeyCount))
	if err != nil {
		seelog.Errorf("Failed to read subkeys of SYSTEM\\CurrentControlSet\\Control\\CCG\\COMClasses with error: %v", err)
		return nil, err
	}

	seelog.Debugf("gMSA Subkeys are %+v", subKeys)
	return subKeys, nil
}

// This function queries all gmsa plugin subkeys to check whether the Amazon ECS Plugin GUID is present.
func isDomainlessGmsaPluginInstalled() (bool, error) {
	subKeys, err := fnQueryDomainlessGmsaPluginSubKeys()
	if err != nil {
		seelog.Errorf("Failed to query gmsa plugin subkeys")
		return false, err
	}

	for _, subKey := range subKeys {
		if subKey == gmsaPluginGUID {
			return true, nil
		}
	}

	return false, nil
}

func parseTaskPidsLimit() int {
	pidsLimitEnvVal := os.Getenv("ECS_TASK_PIDS_LIMIT")
	if pidsLimitEnvVal == "" {
		seelog.Debug("Environment variable empty: ECS_TASK_PIDS_LIMIT")
		return 0
	}
	seelog.Warnf(`"ECS_TASK_PIDS_LIMIT" is not supported on windows`)
	return 0
}
