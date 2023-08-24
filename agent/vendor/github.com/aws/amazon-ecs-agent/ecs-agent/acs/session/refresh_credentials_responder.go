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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
)

const (
	RefreshCredentialsMessageName = "IAMRoleCredentialsMessage"
)

type CredentialsMetadataSetter interface {
	SetTaskRoleCredentialsMetadata(message *ecsacs.IAMRoleCredentialsMessage) error
	SetExecRoleCredentialsMetadata(message *ecsacs.IAMRoleCredentialsMessage) error
}

// refreshCredentialsResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.IAMRoleCredentialsMessage messages sent by ACS.
type refreshCredentialsResponder struct {
	credentialsManager  credentials.Manager
	credsMetadataSetter CredentialsMetadataSetter
	metricsFactory      metrics.EntryFactory
	respond             wsclient.RespondFunc
}

// NewRefreshCredentialsResponder returns an instance of the refreshCredentialsResponder struct.
func NewRefreshCredentialsResponder(credentialsManager credentials.Manager,
	credsMetadataSetter CredentialsMetadataSetter,
	metricsFactory metrics.EntryFactory,
	responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &refreshCredentialsResponder{
		credentialsManager:  credentialsManager,
		credsMetadataSetter: credsMetadataSetter,
		metricsFactory:      metricsFactory,
	}
	r.respond = ResponseToACSSender(r.Name(), responseSender)
	return r
}

func (*refreshCredentialsResponder) Name() string { return "refresh credentials responder" }

func (r *refreshCredentialsResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleCredentialsMessage
}

func (r *refreshCredentialsResponder) handleCredentialsMessage(message *ecsacs.IAMRoleCredentialsMessage) {
	logger.Debug(fmt.Sprintf("Handling %s", RefreshCredentialsMessageName))
	messageID := aws.StringValue(message.MessageId)
	taskARN := aws.StringValue(message.TaskArn)
	metricFields := logger.Fields{
		field.MessageID: messageID,
		field.TaskARN:   taskARN,
	}

	// Validate fields in the message.
	err := validateIAMRoleCredentialsMessage(message)
	if err != nil {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", RefreshCredentialsMessageName),
			logger.Fields{
				field.Error: err,
			})
		err = errors.Wrap(err, "ACS refresh credentials message validation failed")
		r.metricsFactory.New(metrics.CredentialsRefreshFailure).WithFields(metricFields).Done(err)
		return
	}

	// Handle credentials refresh.
	err = r.credentialsManager.SetTaskCredentials(&credentials.TaskIAMRoleCredentials{
		ARN: taskARN,
		IAMRoleCredentials: credentials.IAMRoleCredentialsFromACS(message.RoleCredentials,
			aws.StringValue(message.RoleType)),
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s due to error in setting credentials",
			RefreshCredentialsMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
		err = errors.Wrap(err, "unable to set credentials in the credentials manager")
		r.metricsFactory.New(metrics.CredentialsRefreshFailure).WithFields(metricFields).Done(err)
		return
	}
	err = r.setCredentialsMetadata(message)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s due to error in setting credentials metadata",
			RefreshCredentialsMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
		r.metricsFactory.New(metrics.CredentialsRefreshFailure).WithFields(metricFields).Done(err)
		return
	}

	// Send ACK.
	err = r.respond(&ecsacs.IAMRoleCredentialsAckRequest{
		Expiration:    message.RoleCredentials.Expiration,
		MessageId:     message.MessageId,
		CredentialsId: message.RoleCredentials.CredentialsId,
	})
	if err != nil {
		logger.Warn(fmt.Sprintf("Error acknowledging %s", RefreshCredentialsMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
		err = errors.Wrapf(err, "unable to ACK task credentials for task with ARN %s", taskARN)
		r.metricsFactory.New(metrics.CredentialsRefreshFailure).WithFields(metricFields).Done(err)
		return
	}

	r.metricsFactory.New(metrics.CredentialsRefreshSuccess).WithFields(metricFields).Done(nil)
}

func (r *refreshCredentialsResponder) setCredentialsMetadata(message *ecsacs.IAMRoleCredentialsMessage) error {
	roleType := aws.StringValue(message.RoleType)
	switch roleType {
	case credentials.ApplicationRoleType:
		err := r.credsMetadataSetter.SetTaskRoleCredentialsMetadata(message)
		if err != nil {
			return errors.Wrap(err, "failed to set task role metadata")
		}
	case credentials.ExecutionRoleType:
		err := r.credsMetadataSetter.SetExecRoleCredentialsMetadata(message)
		if err != nil {
			return errors.Wrap(err, "failed to set execution role metadata")
		}
	default:
		return errors.Errorf("received credentials for unexpected roleType \"%s\"", roleType)
	}
	return nil
}

// validateIAMRoleCredentialsMessage performs validation checks on the
// IAMRoleCredentialsMessage.
func validateIAMRoleCredentialsMessage(message *ecsacs.IAMRoleCredentialsMessage) error {
	if message == nil {
		return errors.Errorf("Message is empty")
	}

	messageID := aws.StringValue(message.MessageId)
	if messageID == "" {
		return errors.Errorf("Message ID is not set")
	}

	taskArn := aws.StringValue(message.TaskArn)
	if taskArn == "" {
		return errors.Errorf("taskArn is not set for message ID %s", messageID)
	}

	if message.RoleCredentials == nil {
		return errors.Errorf("roleCredentials is not set for message ID %s", messageID)
	}

	if aws.StringValue(message.RoleCredentials.CredentialsId) == "" {
		return errors.Errorf("roleCredentials ID not set for message ID %s", messageID)
	}

	roleType := aws.StringValue(message.RoleType)
	if !validRoleType(roleType) {
		return errors.Errorf("roleType \"%s\" is invalid for message ID %s with taskArn %s", roleType, messageID,
			taskArn)
	}

	return nil
}

// validRoleType returns false if the RoleType in the ACS refresh credentials message is not
// one of the expected types. Expected types: TaskApplication, TaskExecution
func validRoleType(roleType string) bool {
	switch roleType {
	case credentials.ApplicationRoleType:
		return true
	case credentials.ExecutionRoleType:
		return true
	default:
		return false
	}
}
