//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	httpaws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"errors"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"os"
	"strings"
)

func parseGMSACapability() bool {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), true)
	if envStatus {
		// returns true if the container instance is domain joined
		// this env variable is set in ecs-init module
		return utils.ParseBool(os.Getenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE"), false)
	}
	return false
}

func parseFSxWindowsFileServerCapability() bool {
	return false
}

var IsWindows2016 = func() (bool, error) {
	return false, errors.New("unsupported platform")
}

// GetOSFamily returns "LINUX" as operating system family for linux based ecs instances.
func GetOSFamily() string {
	return strings.ToUpper(OSType)
}
