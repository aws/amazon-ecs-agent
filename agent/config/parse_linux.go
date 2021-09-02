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

import "errors"

func parseGMSACapability() bool {
	return false
}

func parseFSxWindowsFileServerCapability() bool {
	return false
}

var IsWindows2016 = func() (bool, error) {
	return false, errors.New("unsupported platform")
}
