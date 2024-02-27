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

package ecsclient

import (
	"runtime"
)

func getCPUArch() string {
	arch := runtime.GOARCH
	// For amd64 and 386, change to x86_64 and i386 to match the value in instance identity doc
	// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html.
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "386" {
		arch = "i386"
	}
	return arch
}
