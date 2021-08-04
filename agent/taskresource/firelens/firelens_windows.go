// +build windows
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

	"github.com/pkg/errors"
)

var mkdirAll = os.MkdirAll

// createDirectories creates one directory for Windows:
//  - $(DATA_DIR)/firelens/$(TASK_ID)/config: used to store firelens config file. The config file under this directory
//    will be mounted to the firelens container at an expected path.
func (firelens *FirelensResource) createDirectories() error {
	configDir := filepath.Join(firelens.resourceDir, "config")
	err := mkdirAll(configDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create config directory")
	}

	return nil
}
