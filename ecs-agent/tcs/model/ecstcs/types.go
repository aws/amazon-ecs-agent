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
