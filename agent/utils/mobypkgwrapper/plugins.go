// Copyright 2015-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package mobypkgwrapper

import (
	mobyplugins "github.com/docker/docker/pkg/plugins"
)

// Plugins wraps moby/pkg/plugins methods for testing
type Plugins interface {
	Scan() ([]string, error)
}

type plugins struct {
}

// NewPlugins creates a new Plugins object
func NewPlugins() Plugins {
	return &plugins{}
}

func (*plugins) Scan() ([]string, error) {
	return mobyplugins.Scan()
}
