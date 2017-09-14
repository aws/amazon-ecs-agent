// +build windows

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

package resources

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
)

// resources implements the Resources interface
type resources struct{}

// New returns a resources object
func New() Resources {
	return &resources{}
}

// Init is used to initialize the resources
func (r *resources) Init() error {
	return nil
}

// Setup sets up the resources
func (r *resources) Setup(task *api.Task) error {
	return nil
}

// Cleanup removes the resources
func (r *resources) Cleanup(task *api.Task) error {
	return nil
}
