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

package version

import (
	"fmt"
	"runtime"
)

// These are set during build time via -ldflags
var (
	version   string
	buildDate string
)

type VersionInfo struct {
	Version   string `json:"driverVersion"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   version,
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
