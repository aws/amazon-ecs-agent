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

package ioutilwrapper

import (
	"os"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
)

// IOUtil wraps 'os' IO methods for testing
type IOUtil interface {
	TempFile(string, string) (oswrapper.File, error)
	WriteFile(string, []byte, os.FileMode) error
	ReadFile(string) ([]byte, error)
}

type ioUtil struct {
}

// NewIOUtil creates a new IOUtil object
func NewIOUtil() IOUtil {
	return &ioUtil{}
}

func (*ioUtil) TempFile(dir string, prefix string) (oswrapper.File, error) {
	return os.CreateTemp(dir, prefix)
}

func (*ioUtil) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (*ioUtil) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
