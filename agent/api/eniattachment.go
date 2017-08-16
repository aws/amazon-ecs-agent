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

package api

import (
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// ENIAttachment contains the information of the eni attachment
type ENIAttachment struct {
	// TaskARN is the task identifier from ecs
	TaskARN string `json:"taskArn"`
	// AttachmentARN is the identifier for the eni attachment
	AttachmentARN string `json:"attachmentArn"`
	// AttachStatusSent indicates whether the attached status has been sent to backend
	AttachStatusSent bool `json:"attachSent"`
	// MACAddress is the mac address of eni
	MACAddress string `json:"macAddress"`
	// Status is the status of the eni: none/attached/detached
	Status         ENIAttachmentStatus `json:"status"`
	sentStatusLock sync.RWMutex
	// ExpiresAt is the timestamp past which the ENI Attachment is considered
	// unsuccessful. The SubmitTaskStateChange API, with the attachment information
	// should be invoked before this timestamp.
	ExpiresAt time.Time `json:"expiresAt"`
	// ackTimer is used to register the expirtation timeout callback for unsuccessful
	// ENI attachments
	ackTimer ttime.Timer
}

// StartTimer starts the ack timer to record the expiration of ENI attachment
func (eni *ENIAttachment) StartTimer(timeoutFunc func()) error {
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
	seelog.Infof("Starting ENI ack timer for task %s, with mac address %s, to expire at %s, with duration: %s",
		eni.TaskARN, eni.MACAddress, eni.ExpiresAt, duration.String())
	eni.ackTimer = time.AfterFunc(duration, timeoutFunc)
	return nil
}

// IsSent checks if the eni attached status has been sent
func (eni *ENIAttachment) IsSent() bool {
	eni.sentStatusLock.RLock()
	defer eni.sentStatusLock.RUnlock()

	return eni.AttachStatusSent
}

// SetSentStatus marks the eni attached status has been sent
func (eni *ENIAttachment) SetSentStatus() {
	eni.sentStatusLock.Lock()
	defer eni.sentStatusLock.Unlock()

	eni.AttachStatusSent = true
}

// StopAckTimer stops the ack timer set on the ENI attachment
func (eni *ENIAttachment) StopAckTimer() {
	eni.ackTimer.Stop()
}
