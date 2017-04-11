// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ecs_cni

import (
	"os"
	"os/exec"
	"path/filepath"
)

const (
	CNI_PATH       = "/ecs/cni"
	VersionCommand = "--version"
)

type CNIClient interface {
	Version(string) (string, error)
}

// Versions returns the version of the plugin
func Version(name string) (string, error) {
	file := filepath.Join(CNI_PATH, name)
	_, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(file, VersionCommand)
	version, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(version), nil
}
