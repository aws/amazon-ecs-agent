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

package handler

import (
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// ackTimeoutHandler remove ENI attachment from agent state after the ENI ack timeout
type ackTimeoutHandler struct {
	mac        string
	state      dockerstate.TaskEngineState
	dataClient data.Client
}

func (handler *ackTimeoutHandler) handle() {
	eniAttachment, ok := handler.state.ENIByMac(handler.mac)
	if !ok {
		seelog.Warnf("Ignoring unmanaged ENI attachment mac=%s", handler.mac)
		return
	}
	if !eniAttachment.IsSent() {
		seelog.Warnf("Timed out waiting for ENI ack; removing ENI attachment record %s", eniAttachment.String())
		handler.removeENIAttachmentData(handler.mac)
		handler.state.RemoveENIAttachment(handler.mac)
	}
}

func (handler *ackTimeoutHandler) removeENIAttachmentData(mac string) {
	attachmentToRemove, ok := handler.state.ENIByMac(mac)
	if !ok {
		seelog.Errorf("Unable to retrieve ENI Attachment for mac address %s: ", mac)
		return
	}
	attachmentId, err := utils.GetENIAttachmentId(attachmentToRemove.AttachmentARN)
	if err != nil {
		seelog.Errorf("Failed to get attachment id for %s: %v", attachmentToRemove.AttachmentARN, err)
	} else {
		err = handler.dataClient.DeleteENIAttachment(attachmentId)
		if err != nil {
			seelog.Errorf("Failed to remove data for eni attachment %s: %v", attachmentId, err)
		}
	}
}

// sendAck sends ack for a certain ACS message
func sendAck(acsClient wsclient.ClientServer, clusterArn *string, containerInstanceArn *string, messageId *string) {
	if err := acsClient.MakeRequest(&ecsacs.AckRequest{
		Cluster:           clusterArn,
		ContainerInstance: containerInstanceArn,
		MessageId:         messageId,
	}); err != nil {
		seelog.Warnf("Failed to ack request with messageId: %s, error: %v", aws.StringValue(messageId), err)
	}
}

// handleENIAttachment handles an ENI attachment via the following:
// 1. Check whether we already have this attachment in state, if so, start its ack timer and return
// 2. Otherwise add the attachment to state, start its ack timer, and save the state
// These are common tasks for handling a task ENI attachment and an instance ENI attachment, so they are put
// into this function to be shared by both attachment handlers
func handleENIAttachment(attachmentType, attachmentARN, taskARN, mac string,
	expiresAt time.Time,
	state dockerstate.TaskEngineState,
	dataClient data.Client) error {
	seelog.Infof("Handling ENI attachment: %s", attachmentARN)

	if eniAttachment, ok := state.ENIByMac(mac); ok {
		seelog.Infof("Duplicate %s attachment message for ENI mac=%s taskARN=%s attachmentARN=%s",
			attachmentType, mac, taskARN, attachmentARN)
		eniAckTimeoutHandler := ackTimeoutHandler{mac: mac, state: state, dataClient: dataClient}
		return eniAttachment.StartTimer(eniAckTimeoutHandler.handle)
	}
	if err := addENIAttachmentToState(attachmentType, attachmentARN, taskARN, mac, expiresAt, state, dataClient); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("attach %s message handler: unable to add eni attachment to engine state mac=%s taskARN=%s attachmentARN=%s",
			attachmentType, mac, taskARN, attachmentARN))
	}
	return nil
}

// addENIAttachmentToState adds an ENI attachment to state, and start its ack timer
func addENIAttachmentToState(attachmentType, attachmentARN, taskARN, mac string, expiresAt time.Time, state dockerstate.TaskEngineState, dataClient data.Client) error {
	eniAttachment := &apieni.ENIAttachment{
		TaskARN:          taskARN,
		AttachmentType:   attachmentType,
		AttachmentARN:    attachmentARN,
		AttachStatusSent: false,
		MACAddress:       mac,
		ExpiresAt:        expiresAt, // Stop tracking the eni attachment after timeout
	}
	eniAckTimeoutHandler := ackTimeoutHandler{mac: mac, state: state, dataClient: dataClient}
	if err := eniAttachment.StartTimer(eniAckTimeoutHandler.handle); err != nil {
		return err
	}

	switch attachmentType {
	case apieni.ENIAttachmentTypeTaskENI:
		taskId, _ := utils.TaskIdFromArn(taskARN)
		logger.Info("Adding eni attachment info to state for task", logger.Fields{
			field.TaskID:    taskId,
			"attachmentARN": attachmentARN,
			"mac":           mac,
		})
	case apieni.ENIAttachmentTypeInstanceENI:
		logger.Info("Adding instance eni attachment info to state", logger.Fields{
			"attachmentARN": attachmentARN,
			"mac":           mac,
		})
	default:
		return fmt.Errorf("unrecognized eni attachment type: %s", attachmentType)
	}
	state.AddENIAttachment(eniAttachment)
	if err := dataClient.SaveENIAttachment(eniAttachment); err != nil {
		logger.Error("Failed to save data for eni attachment", logger.Fields{
			"attachmentARN": attachmentARN,
			field.Error:     err,
		})
	}
	return nil
}
