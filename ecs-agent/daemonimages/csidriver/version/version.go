// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
