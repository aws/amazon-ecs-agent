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
	"fmt"

	_ "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgm"
)

const (
	// TerminalFailureAgentExitCode is the exit code used when dcgm-init hits a
	// terminal error and should not be restarted.
	TerminalFailureAgentExitCode = 5
	// DefaultInitErrorExitCode is the exit code used for general init errors.
	DefaultInitErrorExitCode = -1
)

// errNotImplemented is returned by the stub engine methods until the metrics
// collection logic is implemented.
var errNotImplemented = errors.New("not implemented")

// Engine drives the dcgm-init metrics collection loop.
type Engine struct{}

// TerminalError indicates a failure that should not be retried.
type TerminalError struct {
	err      string
	exitCode int
}

func (e *TerminalError) Error() string {
	return fmt.Sprintf("%s: %d", e.err, e.exitCode)
}

// New creates an instance of Engine.
func New() (*Engine, error) {
	return &Engine{}, nil
}

// Start begins collecting GPU metrics.
func (e *Engine) Start() error {
	return errNotImplemented
}

// Stop stops collecting GPU metrics.
func (e *Engine) Stop() error {
	return errNotImplemented
}
