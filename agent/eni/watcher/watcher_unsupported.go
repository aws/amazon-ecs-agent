//go:build !linux && !windows
// +build !linux,!windows

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

package watcher

import (
	"context"
	"errors"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

type ENIWatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	updateIntervalTicker *time.Ticker
	agentState           dockerstate.TaskEngineState
	eniChangeEvent       chan<- statechange.Event
	primaryMAC           string
}

// newWatcher is used to nest the return of the ENIWatcher struct
func newWatcher(ctx context.Context,
	primaryMAC string,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*ENIWatcher, error) {
	return nil, errors.New("unsupported platform")
}

func (eniWatcher *ENIWatcher) reconcileOnce(withRetry bool) error {
	return nil
}

func (eniWatcher *ENIWatcher) eventHandler() {
}
