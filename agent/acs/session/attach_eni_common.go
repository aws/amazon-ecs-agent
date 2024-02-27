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

package session

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/arn"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// eniHandler struct implements ENIHandler interface defined in ecs-agent module.
// It removes ENI attachment from agent state after the ENI ack timeout.
type eniHandler struct {
	mac        string
	state      dockerstate.TaskEngineState
	dataClient data.Client
}

// NewENIHandler creates a new eniHandler.
func NewENIHandler(state dockerstate.TaskEngineState, dataClient data.Client) *eniHandler {
	return &eniHandler{
		state:      state,
		dataClient: dataClient,
	}
}

// HandleENIAttachment handles an ENI attachment via the following:
// 1. Check whether we already have this attachment in state, if so, start its ack timer and return
// 2. Otherwise add the attachment to state, start its ack timer, and save the state
// These are common tasks for handling a task ENI attachment and an instance ENI attachment, so they are put
// into this function to be shared by both attachment handlers
func (eniHandler *eniHandler) HandleENIAttachment(ea *ni.ENIAttachment) error {
	attachmentType := ea.AttachmentType
	attachmentARN := ea.AttachmentARN
	taskARN := ea.TaskARN
	mac := ea.MACAddress
	eniHandler.mac = mac

	seelog.Infof("Handling ENI attachment: %s", attachmentARN)

	if eniAttachment, ok := eniHandler.state.ENIByMac(mac); ok {
		seelog.Infof("Duplicate %s attachment message for ENI mac=%s taskARN=%s attachmentARN=%s",
			attachmentType, mac, taskARN, attachmentARN)
		return eniAttachment.StartTimer(eniHandler.handleENIAckTimeout)
	}
	if err := eniHandler.addENIAttachmentToState(ea); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("attach %s message handler: unable to add eni attachment to engine state mac=%s taskARN=%s attachmentARN=%s",
			attachmentType, mac, taskARN, attachmentARN))
	}
	return nil
}

// addENIAttachmentToState adds an ENI attachment to state, and start its ack timer
func (eniHandler *eniHandler) addENIAttachmentToState(ea *ni.ENIAttachment) error {
	attachmentType := ea.AttachmentType
	attachmentARN := ea.AttachmentARN
	taskARN := ea.TaskARN
	mac := ea.MACAddress
	eniHandler.mac = mac

	if err := ea.StartTimer(eniHandler.handleENIAckTimeout); err != nil {
		return err
	}

	switch attachmentType {
	case ni.ENIAttachmentTypeTaskENI:
		taskId, _ := arn.TaskIdFromArn(taskARN)
		logger.Info("Adding eni attachment info to state for task", logger.Fields{
			field.TaskID:    taskId,
			"attachmentARN": attachmentARN,
			"mac":           mac,
		})
	case ni.ENIAttachmentTypeInstanceENI:
		logger.Info("Adding instance eni attachment info to state", logger.Fields{
			"attachmentARN": attachmentARN,
			"mac":           mac,
		})
	default:
		return fmt.Errorf("unrecognized eni attachment type: %s", attachmentType)
	}
	eniHandler.state.AddENIAttachment(ea)
	if err := eniHandler.dataClient.SaveENIAttachment(ea); err != nil {
		logger.Error("Failed to save data for eni attachment", logger.Fields{
			"attachmentARN": attachmentARN,
			field.Error:     err,
		})
	}
	return nil
}

// handleENIAckTimeout removes ENI attachment from agent state after the ENI ack timeout
func (eniHandler *eniHandler) handleENIAckTimeout() {
	eniAttachment, ok := eniHandler.state.ENIByMac(eniHandler.mac)
	if !ok {
		seelog.Warnf("Ignoring unmanaged ENI attachment mac=%s", eniHandler.mac)
		return
	}
	if !eniAttachment.IsSent() {
		seelog.Warnf("Timed out waiting for ENI ack; removing ENI attachment record %s", eniAttachment.String())
		eniHandler.removeENIAttachmentData(eniHandler.mac)
		eniHandler.state.RemoveENIAttachment(eniHandler.mac)
	}
}

func (eniHandler *eniHandler) removeENIAttachmentData(mac string) {
	attachmentToRemove, ok := eniHandler.state.ENIByMac(mac)
	if !ok {
		seelog.Errorf("Unable to retrieve ENI Attachment for mac address %s: ", mac)
		return
	}
	attachmentId, err := utils.GetAttachmentId(attachmentToRemove.AttachmentARN)
	if err != nil {
		seelog.Errorf("Failed to get attachment id for %s: %v", attachmentToRemove.AttachmentARN, err)
	} else {
		err = eniHandler.dataClient.DeleteENIAttachment(attachmentId)
		if err != nil {
			seelog.Errorf("Failed to remove data for eni attachment %s: %v", attachmentId, err)
		}
	}
}
