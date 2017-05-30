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
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	log "github.com/cihub/seelog"
	"github.com/vishvananda/netlink"
)

// StateManager defines the method to manage the state of eni
type StateManager interface {
	Init(state []netlink.Link)
	Reconcile(currentState map[string]string)
	HandleENIEvent(mac string)
}

// stateManager handles the state change of eni
type stateManager struct {
	agentState dockerstate.TaskEngineState
	taskEvent  chan api.TaskStateChange
}

// New returns a new StateManager
func New(state dockerstate.TaskEngineState, event chan api.TaskStateChange) StateManager {
	return &stateManager{
		agentState: state,
		taskEvent:  event,
	}
}

// Init populates the initial state of the map
func (statemanager *stateManager) Init(state []netlink.Link) {
	for _, link := range state {
		macAddress := link.Attrs().HardwareAddr.String()
		statemanager.HandleENIEvent(macAddress)
	}
}

// ENIStateChangeShouldBeSent checks whether this eni is managed by ecs
// and if its status should be sent to backend
func (statemanager *stateManager) ENIStateChangeShouldBeSent(macAddress string) (*api.ENIAttachment, bool) {
	if macAddress == "" {
		log.Warn("ENI state manager: device with empty mac address")
		return nil, false
	}
	// check if this is an eni required by a task
	eni, ok := statemanager.agentState.ENIByMac(macAddress)
	if !ok {
		log.Infof("ENI state manager: eni not managed by ecs: %s", macAddress)
		return nil, false
	}

	if eni.GetSentStatus() {
		log.Infof("ENI state manager: eni attach status has already sent: %s", macAddress)
		return eni, false
	}

	return eni, true
}

// HandleENIEvent handles the eni event from udev or reconcil phase
func (statemanager *stateManager) HandleENIEvent(mac string) {
	eni, ok := statemanager.ENIStateChangeShouldBeSent(mac)
	if ok {
		eni.Status = api.ENIAttached
		statemanager.emitENIAttachmentEvent(api.TaskStateChange{
			TaskArn:     eni.TaskArn,
			Attachments: eni,
		})
	}
}

// emitENIAttachmentEvent send the eni statechange(attach) to event handler
func (statemanager *stateManager) emitENIAttachmentEvent(event api.TaskStateChange) {
	log.Infof("ENI state manager: sending eni state change to event handler: %v", event)
	statemanager.taskEvent <- event
}

// Reconcile performs a 2 phase reconciliation of managed state
func (statemanager *stateManager) Reconcile(currentState map[string]string) {
	// Add new interfaces next
	for mac, _ := range currentState {
		statemanager.HandleENIEvent(mac)
	}
}
