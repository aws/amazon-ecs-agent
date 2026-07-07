//go:build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"errors"

	_ "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgm"
)

const (
	// DefaultInitErrorExitCode is the exit code used for general init errors.
	DefaultInitErrorExitCode = -1
)

// errNotImplemented is returned by the stub engine methods until the metrics
// collection logic is implemented.
var errNotImplemented = errors.New("not implemented")

// Engine drives the dcgm-init metrics collection loop.
type Engine struct{}

// New creates an instance of Engine.
func New() (*Engine, error) {
	return &Engine{}, nil
}

// Start begins collecting GPU metrics.
func (e *Engine) Start() error {
	return errNotImplemented
}
