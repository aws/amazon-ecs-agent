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

import "golang.org/x/sys/windows/registry"

const OSType = "windows"
const (
	// envSkipDomainJoinCheck is an environment setting that can be used to skip
	// domain join check validation. This is useful for integration and
	// functional-tests but should not be set for any non-test use-case.
	envSkipDomainJoinCheck  = "ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION"
	releaseId2004SAC        = "2004"
	releaseId1909SAC        = "1909"
	windowsServer2019       = "Windows Server 2019"
	windowsServer2016       = "Windows Server 2016"
	windowsServerDataCenter = "Windows Server Datacenter"
	installationTypeCore    = "Server Core"
	installationTypeFull    = "Server"
	unsupportedWindowsOS    = "windows"
	osTypeFormat            = "WINDOWS_SERVER_%s_%s"
	ecsWinRegistryRootKey   = registry.LOCAL_MACHINE
	ecsWinRegistryRootPath  = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
)
