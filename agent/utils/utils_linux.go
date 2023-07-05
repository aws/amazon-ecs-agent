//go:build linux
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

package utils

import (
	"fmt"
	"io/fs"
	"os"
)

func GetCanonicalPath(path string) string { return path }

func MkdirAllAndChown(path string, perm fs.FileMode, uid, gid int) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, perm)
	}
	if err != nil {
		return fmt.Errorf("failed to mkdir %s: %+v", path, err)
	}
	if err = os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("failed to chown %s: %+v", path, err)
	}
	return nil
}
