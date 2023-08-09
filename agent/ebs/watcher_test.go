//go:build unit
// +build unit

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

package ebs

import (
	"context"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

func setupWatcher(ctx context.Context, cancel context.CancelFunc, agentState dockerstate.TaskEngineState,
	ebsChangeEvent chan<- statechange.Event) *EBSWatcher {
	return &EBSWatcher{
		ctx:            ctx,
		cancel:         cancel,
		agentState:     agentState,
		ebsChangeEvent: ebsChangeEvent,
		mailbox:        make(chan func(), 1),
	}
}
