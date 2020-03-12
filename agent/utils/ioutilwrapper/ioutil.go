// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package ioutilwrapper

import (
	"io/ioutil"
	"os"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
)

// IOUtil wraps 'io/util' methods for testing
type IOUtil interface {
	TempFile(string, string) (oswrapper.File, error)
	WriteFile(string, []byte, os.FileMode) error
}

type ioUtil struct {
}

// NewIOUtil creates a new IOUtil object
func NewIOUtil() IOUtil {
	return &ioUtil{}
}

func (*ioUtil) TempFile(dir string, prefix string) (oswrapper.File, error) {
	return ioutil.TempFile(dir, prefix)
}

func (*ioUtil) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}
