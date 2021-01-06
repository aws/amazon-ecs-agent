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

package eni

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// ENIAttachmentTypeTaskENI represents the type of a task level eni
	ENIAttachmentTypeTaskENI = "task-eni"
	// ENIAttachmentTypeInstanceENI represents the type of an instance level eni
	ENIAttachmentTypeInstanceENI = "instance-eni"
)

// ENIAttachment contains the information of the eni attachment
type ENIAttachment struct {
	// AttachmentType is the type of the eni attachment, can either be "task-eni" or "instance-eni"
	AttachmentType string `json:"attachmentType"`
	// TaskARN is the task identifier from ecs
	TaskARN string `json:"taskArn"`
	// AttachmentARN is the identifier for the eni attachment
	AttachmentARN string `json:"attachmentArn"`
	// AttachStatusSent indicates whether the attached status has been sent to backend
	AttachStatusSent bool `json:"attachSent"`
	// MACAddress is the mac address of eni
	MACAddress string `json:"macAddress"`
	// Status is the status of the eni: none/attached/detached
	Status ENIAttachmentStatus `json:"status"`
	// ExpiresAt is the timestamp past which the ENI Attachment is considered
	// unsuccessful. The SubmitTaskStateChange API, with the attachment information
	// should be invoked before this timestamp.
	ExpiresAt time.Time `json:"expiresAt"`
	// ackTimer is used to register the expirtation timeout callback for unsuccessful
	// ENI attachments
	ackTimer ttime.Timer
	// guard protects access to fields of this struct
	guard sync.RWMutex
}

// StartTimer starts the ack timer to record the expiration of ENI attachment
func (eni *ENIAttachment) StartTimer(timeoutFunc func()) error {
	eni.guard.Lock()
	defer eni.guard.Unlock()

	if eni.ackTimer != nil {
		// The timer has already been initialized, do nothing
		return nil
	}
	now := time.Now()
	duration := eni.ExpiresAt.Sub(now)
	if duration <= 0 {
		return errors.Errorf("eni attachment: timer expiration is in the past; expiration [%s] < now [%s]",
			eni.ExpiresAt.String(), now.String())
	}
	seelog.Infof("Starting ENI ack timer with duration=%s, %s", duration.String(), eni.stringUnsafe())
	eni.ackTimer = time.AfterFunc(duration, timeoutFunc)
	return nil
}

// Initialize initializes the fields that can't be populated from loading state file.
// Notably, this initializes the ack timer so that if we times out waiting for the eni to be attached, the attachment
// can be removed from state.
func (eni *ENIAttachment) Initialize(timeoutFunc func()) error {
	eni.guard.Lock()
	defer eni.guard.Unlock()

	if eni.AttachStatusSent { // eni attachment status has been sent, no need to start ack timer.
		return nil
	}

	now := time.Now()
	duration := eni.ExpiresAt.Sub(now)
	if duration <= 0 {
		return errors.New("ENI attachment has already expired")
	}

	seelog.Infof("Starting ENI ack timer with duration=%s, %s", duration.String(), eni.stringUnsafe())
	eni.ackTimer = time.AfterFunc(duration, timeoutFunc)
	return nil
}

// IsSent checks if the eni attached status has been sent
func (eni *ENIAttachment) IsSent() bool {
	eni.guard.RLock()
	defer eni.guard.RUnlock()

	return eni.AttachStatusSent
}

// SetSentStatus marks the eni attached status has been sent
func (eni *ENIAttachment) SetSentStatus() {
	eni.guard.Lock()
	defer eni.guard.Unlock()

	eni.AttachStatusSent = true
}

// StopAckTimer stops the ack timer set on the ENI attachment
func (eni *ENIAttachment) StopAckTimer() {
	eni.guard.Lock()
	defer eni.guard.Unlock()

	eni.ackTimer.Stop()
}

// HasExpired returns true if the ENI attachment object has exceeded the
// threshold for notifying the backend of the attachment
func (eni *ENIAttachment) HasExpired() bool {
	eni.guard.RLock()
	defer eni.guard.RUnlock()

	return time.Now().After(eni.ExpiresAt)
}

// String returns a string representation of the ENI Attachment
func (eni *ENIAttachment) String() string {
	eni.guard.RLock()
	defer eni.guard.RUnlock()

	return eni.stringUnsafe()
}

// stringUnsafe returns a string representation of the ENI Attachment
func (eni *ENIAttachment) stringUnsafe() string {
	// skip TaskArn field for instance level eni attachment since it won't have a task arn
	if eni.AttachmentType == ENIAttachmentTypeInstanceENI {
		return fmt.Sprintf(
			"ENI Attachment: attachment=%s attachmentType=%s attachmentSent=%t mac=%s status=%s expiresAt=%s",
			eni.AttachmentARN, eni.AttachmentType, eni.AttachStatusSent, eni.MACAddress, eni.Status.String(), eni.ExpiresAt.Format(time.RFC3339))
	}

	return fmt.Sprintf(
		"ENI Attachment: task=%s attachment=%s attachmentType=%s attachmentSent=%t mac=%s status=%s expiresAt=%s",
		eni.TaskARN, eni.AttachmentARN, eni.AttachmentType, eni.AttachStatusSent, eni.MACAddress, eni.Status.String(), eni.ExpiresAt.Format(time.RFC3339))
}
