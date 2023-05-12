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
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

func HeartbeatHandlerFunc(acsClient wsclient.ClientServer, doctor *doctor.Doctor) func(message *ecsacs.HeartbeatMessage) {
	return func(message *ecsacs.HeartbeatMessage) {
		handleSingleHeartbeatMessage(acsClient, doctor, message)
	}
}

// To handle a Heartbeat Message the doctor health checks need to be run and
// an ACK needs to be sent back to ACS.

// This function is meant to be called from the ACS dispatcher and as such
// should not block in any way to prevent starvation of the message handler
func handleSingleHeartbeatMessage(acsClient wsclient.ClientServer, doctor *doctor.Doctor, message *ecsacs.HeartbeatMessage) {
	// Agent will run healthchecks triggered by ACS heartbeat
	// healthcheck results will be sent on to TACS, but for now just to debug logs.
	go doctor.RunHealthchecks()

	// Agent will send simple ack
	ack := &ecsacs.HeartbeatAckRequest{
		MessageId: message.MessageId,
	}
	go func() {
		err := acsClient.MakeRequest(ack)
		if err != nil {
			seelog.Warnf("Error acknowledging server heartbeat, message id: %s, error: %s", aws.StringValue(ack.MessageId), err)
		}
	}()
}
