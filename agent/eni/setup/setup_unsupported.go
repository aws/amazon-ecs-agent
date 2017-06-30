// +build !linux

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

package setup

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	eniWatcher "github.com/aws/amazon-ecs-agent/agent/eni"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

// udevWatcher is a local construct to mimic the eniwatcher on unsupported platforms
type udevWatcher struct{}

// New is used to return an error for unsupported platforms
func New(ctx context.Context, state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) (*eniWatcher.UdevWatcher, error) {
	return nil, errors.New(fmt.Sprintf("eni watcher new: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH))
}
