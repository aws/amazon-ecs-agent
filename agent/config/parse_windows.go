// +build windows

// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"syscall"
	"unsafe"

	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// parseGMSACapability is used to determine if gMSA support can be enabled
func parseGMSACapability() bool {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), true)
	if envStatus {
		// If gMSA feature is explicitly enabled, check if container instance is domain joined.
		// If container instance is not domain joined, explicitly disable feature configuration.
		status, err := isDomainJoined()
		if err != nil || status != true {
			return false
		}
	}

	return true
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

	syscall.NetApiBufferFree((*byte)(unsafe.Pointer(domain)))

	return status == syscall.NetSetupDomainName, nil
}
