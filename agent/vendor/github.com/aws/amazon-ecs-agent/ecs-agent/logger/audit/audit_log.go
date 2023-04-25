// copyright amazon.com inc. or its affiliates. all rights reserved.
//
// licensed under the apache license, version 2.0 (the "license"). you may
// not use this file except in compliance with the license. a copy of the
// license is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. this file is distributed
// on an "as is" basis, without warranties or conditions of any kind, either
// express or implied. see the license for the specific language governing
// permissions and limitations under the license.

//go:generate mockgen -destination=mocks/mock_audit_logger.go -copyright_file=../../../scripts/copyright_file . AuditLogger

package audit

import "github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/request"

type AuditLogger interface {
	Log(r request.LogRequest, httpResponseCode int, eventType string)
	GetContainerInstanceArn() string
	GetCluster() string
}
