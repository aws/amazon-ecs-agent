//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

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

package fs

import (
	"fmt"
)

// Info unsupported returns 0 values for available and capacity and an error.
func Info(path string) (int64, int64, int64, int64, int64, int64, error) {
	return 0, 0, 0, 0, 0, 0, fmt.Errorf("fsinfo not supported for this build")
}
