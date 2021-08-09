// +build linux
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

package firelens

import (
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
)

func (firelens *FirelensResource) createSocketDirectories() error {
	socketDir := filepath.Join(firelens.resourceDir, "socket")
	err := mkdirAll(socketDir, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

var rename = os.Rename

// writeConfigFile writes a config file at a given path.
func (firelens *FirelensResource) writeConfigFile(writeFunc func(file oswrapper.File) error, filePath string) error {
	temp, err := firelens.ioutil.TempFile(firelens.resourceDir, tempFile)
	if err != nil {
		return err
	}
	defer temp.Close()

	err = writeFunc(temp)
	if err != nil {
		return err
	}

	err = temp.Chmod(os.FileMode(configFilePerm))
	if err != nil {
		return err
	}

	// Persist the config file to disk.
	err = temp.Sync()
	if err != nil {
		return err
	}

	err = rename(temp.Name(), filePath)
	if err != nil {
		return err
	}

	return nil
}
