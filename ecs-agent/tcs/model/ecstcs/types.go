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
