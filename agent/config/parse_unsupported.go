//go:build !linux && !windows
// +build !linux,!windows

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
	"strings"
)

func parseGMSACapability() BooleanDefaultFalse {
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
}

func parseFSxWindowsFileServerCapability() BooleanDefaultTrue {
	return BooleanDefaultTrue{Value: ExplicitlyDisabled}
}

func parseGMSADomainlessCapability() BooleanDefaultFalse {
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
}

var IsWindows2016 = func() (bool, error) {
	return false, errors.New("unsupported platform")
}

// GetOSFamily returns "UNSUPPORTED" as operating system family for non-windows based ecs instances.
func GetOSFamily() string {
	return strings.ToUpper(OSType)
}

func parseTaskPidsLimit() int {
	return 0
}
