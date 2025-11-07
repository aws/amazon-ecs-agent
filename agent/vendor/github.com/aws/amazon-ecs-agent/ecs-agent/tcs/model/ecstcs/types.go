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

package ecstcs

import (
	"errors"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// NewPublishMetricsRequest creates a PublishMetricsRequest object.
func NewPublishMetricsRequest(instanceMetrics *InstanceMetrics, metadata *MetricsMetadata, taskMetrics []*TaskMetric) *PublishMetricsRequest {
	return &PublishMetricsRequest{
		InstanceMetrics: instanceMetrics,
		Metadata:        metadata,
		TaskMetrics:     taskMetrics,
		Timestamp:       (*utils.Timestamp)(aws.Time(time.Now())),
	}
}

// NewPublishHealthMetricsRequest creates a PublishHealthRequest
func NewPublishHealthMetricsRequest(metadata *HealthMetadata, healthMetrics []*TaskHealth) *PublishHealthRequest {
	return &PublishHealthRequest{
		Metadata:  metadata,
		Tasks:     healthMetrics,
		Timestamp: (*utils.Timestamp)(aws.Time(time.Now())),
	}
}

type TelemetryMessage struct {
	InstanceMetrics *InstanceMetrics
	Metadata        *MetricsMetadata
	TaskMetrics     []*TaskMetric
}

type HealthMessage struct {
	Metadata      *HealthMetadata
	HealthMetrics []*TaskHealth
}

// InstanceStatusMessage represents a message containing instance health status
// information to be published to the TCS backend. This message type follows
// the same pattern as TelemetryMessage and HealthMessage, providing a structured
// way to send instance status updates through a dedicated channel.
//
// The message contains metadata about the container instance and a collection
// of status checks that indicate the health of various components on the instance.
// This allows external components to send instance status updates independently
// of the doctor module's periodic health checks.
type InstanceStatusMessage struct {
	// Metadata contains identifying information about the container instance
	// including cluster name, container instance ARN, and request ID.
	Metadata *InstanceStatusMetadata `json:"metadata,omitempty"`

	// Statuses contains a collection of instance status checks that represent
	// the health state of various components on the container instance.
	Statuses []*InstanceStatus `json:"statuses,omitempty"`
}

const (
	// HealthcheckStatusInitializing is the zero state of a healthcheck status.
	HealthcheckStatusInitializing HealthcheckStatus = iota
	// HealthcheckStatusOk represents a healthcheck with a true/success result.
	HealthcheckStatusOk
	// HealthcheckStatusImpaired represents a healthcheck with a false/fail result.
	HealthcheckStatusImpaired
)

// HealthcheckStatus is an enumeration of possible instance statuses.
type HealthcheckStatus int32

var healthcheckStatusMap = map[string]HealthcheckStatus{
	"INITIALIZING": HealthcheckStatusInitializing,
	"OK":           HealthcheckStatusOk,
	"IMPAIRED":     HealthcheckStatusImpaired,
}

// String returns a human readable string representation of this object.
func (hs HealthcheckStatus) String() string {
	for k, v := range healthcheckStatusMap {
		if v == hs {
			return k
		}
	}
	// We shouldn't see this.
	return "NONE"
}

// Ok returns true if the Healthcheck status is OK or INITIALIZING.
func (hs HealthcheckStatus) Ok() bool {
	return hs == HealthcheckStatusOk || hs == HealthcheckStatusInitializing
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded HealthcheckStatus data.
func (hs *HealthcheckStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*hs = HealthcheckStatusInitializing
		return nil
	}
	if b[0] != '"' || b[len(b)-1] != '"' {
		*hs = HealthcheckStatusInitializing
		return errors.New("healthcheck status unmarshal: status must be a string or null; Got " + string(b))
	}

	stat, ok := healthcheckStatusMap[string(b[1:len(b)-1])]
	if !ok {
		*hs = HealthcheckStatusInitializing
		return errors.New("healthcheck status unmarshal: unrecognized status")
	}
	*hs = stat
	return nil
}

// MarshalJSON overrides the logic for JSON-encoding the HealthcheckStatus type.
func (hs *HealthcheckStatus) MarshalJSON() ([]byte, error) {
	if hs == nil {
		return nil, nil
	}
	return []byte(`"` + hs.String() + `"`), nil
}
