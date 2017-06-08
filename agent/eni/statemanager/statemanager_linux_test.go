// +build linux

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

package statemanager

import (
	"sync"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/stretchr/testify/assert"
)

// TestHandleENIEvent tests HandleENIEvent code path
func TestHandleENIEvent(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	stateManager := New(taskEngineState, eventChannel)

	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       "mac1",
		AttachStatusSent: false,
	})

	var event statechange.Event
	go func() {
		event = <-eventChannel
	}()

	stateManager.HandleENIEvent("mac1")
	assert.Equal(t, "mac1", event.(api.TaskStateChange).Attachments.MacAddress)
}

// TestENIStateShouldBeSent tests the state change of ENI should be sent
func TestENIStateShouldBeSent(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	statemanager := New(taskEngineState, eventChannel)

	eniOfECS := &api.ENIAttachment{
		MacAddress:       "mac1",
		AttachStatusSent: false,
	}
	taskEngineState.AddENIAttachment(eniOfECS)

	eni, ok := statemanager.(*stateManager).ENIStateChangeShouldBeSent("mac1")
	assert.True(t, ok)
	assert.Equal(t, eniOfECS, eni)
}

// TestENINotManagedByECSShouldNotSent tests status change of eni not managed
// by ecs shouldn't be sent
func TestENINotManagedByECSShouldNotSent(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	statemanager := New(taskEngineState, eventChannel)

	eni, ok := statemanager.(*stateManager).ENIStateChangeShouldBeSent("mac_not_existed")
	assert.Nil(t, eni)
	assert.False(t, ok)
}

// TestENIHasSentShouldNotSent tests duplicate eni attachment status shouldn't be sent
func TestENIHasSentShouldNotSent(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	statemanager := New(taskEngineState, eventChannel)

	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       "mac1",
		AttachStatusSent: true,
	})

	eni, ok := statemanager.(*stateManager).ENIStateChangeShouldBeSent("mac1")
	assert.False(t, ok)
	assert.Equal(t, "mac1", eni.MacAddress)
}

// TestReconcile tests the Reconcile code path
func TestReconcile(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       "mac1",
		AttachStatusSent: false,
	})
	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       "mac2",
		AttachStatusSent: false,
	})

	eventChannel := make(chan statechange.Event)

	// Expect two events of statechange
	var waitStateChangeEvent sync.WaitGroup
	waitStateChangeEvent.Add(2)
	done := make(chan struct{})

	statemanager := New(taskEngineState, eventChannel)
	currentState := make(map[string]string)
	currentState["mac1"] = "eth0"
	currentState["mac2"] = "eth1"

	go func() {
		for {
			select {
			case <-eventChannel:
				waitStateChangeEvent.Done()
			case <-done:
				return
			}
		}
	}()

	statemanager.Reconcile(currentState)

	waitStateChangeEvent.Wait()
	done <- struct{}{}
}

// TestEmitAttachmentEvent tests the statechange event was correctly sent
func TestEmitAttachmentEvent(t *testing.T) {
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)
	statemanager := New(taskEngineState, eventChannel)

	event := api.TaskStateChange{
		Attachments: &api.ENIAttachment{
			MacAddress: "mac",
		},
	}
	invoked := make(chan struct{})
	var eventReceived statechange.Event
	go func() {
		eventReceived = <-eventChannel
		invoked <- struct{}{}
	}()

	statemanager.(*stateManager).emitENIAttachmentEvent(event)
	<-invoked

	assert.NotNil(t, eventReceived.(api.TaskStateChange).Attachments)
	assert.Equal(t, "mac", eventReceived.(api.TaskStateChange).Attachments.MacAddress)
}
