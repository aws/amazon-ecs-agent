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

//go:generate mockgen -destination=mocks/mock_audit_logger.go -copyright_file=../../../scripts/copyright_file . AuditLogger

package audit

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/request"
)

const (
	GetCredentialsEventType                = "GetCredentials"
	GetCredentialsTaskExecutionEventType   = "GetCredentialsExecutionRole"
	GetCredentialsInvalidRoleTypeEventType = "GetCredentialsInvalidRoleType"
)

type AuditLogger interface {
	Log(r request.LogRequest, httpResponseCode int, eventType string)
	GetContainerInstanceArn() string
	GetCluster() string
}

// Returns a suitable audit log event type for the credentials role type
func GetCredentialsEventTypeFromRoleType(roleType string) string {
	switch roleType {
	case credentials.ApplicationRoleType:
		return GetCredentialsEventType
	case credentials.ExecutionRoleType:
		return GetCredentialsTaskExecutionEventType
	default:
		return GetCredentialsInvalidRoleTypeEventType
	}
}
