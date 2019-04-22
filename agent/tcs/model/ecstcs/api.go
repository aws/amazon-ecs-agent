// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	"github.com/aws/aws-sdk-go/aws/awsutil"
)

type AckPublishHealth struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s AckPublishHealth) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AckPublishHealth) GoString() string {
	return s.String()
}

type AckPublishMetric struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s AckPublishMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AckPublishMetric) GoString() string {
	return s.String()
}

type BadRequestException struct {
	_ struct{} `type:"structure"`

	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s BadRequestException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s BadRequestException) GoString() string {
	return s.String()
}

type CWStatsSet struct {
	_ struct{} `type:"structure"`

	Max *float64 `locationName:"max" type:"double"`

	Min *float64 `locationName:"min" type:"double"`

	SampleCount *int64 `locationName:"sampleCount" type:"integer"`

	Sum *float64 `locationName:"sum" type:"double"`
}

// String returns the string representation
func (s CWStatsSet) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CWStatsSet) GoString() string {
	return s.String()
}

type ContainerHealth struct {
	_ struct{} `type:"structure"`

	ContainerName *string `locationName:"containerName" type:"string"`

	HealthStatus *string `locationName:"healthStatus" type:"string" enum:"HealthStatus"`

	StatusSince *time.Time `locationName:"statusSince" type:"timestamp"`
}

// String returns the string representation
func (s ContainerHealth) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerHealth) GoString() string {
	return s.String()
}

type ContainerMetric struct {
	_ struct{} `type:"structure"`

	CpuStatsSet *CWStatsSet `locationName:"cpuStatsSet" type:"structure"`

	MemoryStatsSet *CWStatsSet `locationName:"memoryStatsSet" type:"structure"`
}

// String returns the string representation
func (s ContainerMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerMetric) GoString() string {
	return s.String()
}

type HealthMetadata struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	Fin *bool `locationName:"fin" type:"boolean"`

	MessageId *string `locationName:"messageId" type:"string"`
}

// String returns the string representation
func (s HealthMetadata) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HealthMetadata) GoString() string {
	return s.String()
}

type HeartbeatInput struct {
	_ struct{} `type:"structure"`

	Healthy *bool `locationName:"healthy" type:"boolean"`
}

// String returns the string representation
func (s HeartbeatInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HeartbeatInput) GoString() string {
	return s.String()
}

type HeartbeatMessage struct {
	_ struct{} `type:"structure"`

	Healthy *bool `locationName:"healthy" type:"boolean"`
}

// String returns the string representation
func (s HeartbeatMessage) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HeartbeatMessage) GoString() string {
	return s.String()
}

type HeartbeatOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s HeartbeatOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HeartbeatOutput) GoString() string {
	return s.String()
}

type InvalidParameterException struct {
	_ struct{} `type:"structure"`

	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s InvalidParameterException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InvalidParameterException) GoString() string {
	return s.String()
}

type MetricsMetadata struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	Fin *bool `locationName:"fin" type:"boolean"`

	Idle *bool `locationName:"idle" type:"boolean"`

	MessageId *string `locationName:"messageId" type:"string"`
}

// String returns the string representation
func (s MetricsMetadata) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s MetricsMetadata) GoString() string {
	return s.String()
}

type PublishHealthInput struct {
	_ struct{} `type:"structure"`

	Metadata *HealthMetadata `locationName:"metadata" type:"structure"`

	Tasks []*TaskHealth `locationName:"tasks" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp"`
}

// String returns the string representation
func (s PublishHealthInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishHealthInput) GoString() string {
	return s.String()
}

type PublishHealthOutput struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s PublishHealthOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishHealthOutput) GoString() string {
	return s.String()
}

type PublishHealthRequest struct {
	_ struct{} `type:"structure"`

	Metadata *HealthMetadata `locationName:"metadata" type:"structure"`

	Tasks []*TaskHealth `locationName:"tasks" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp"`
}

// String returns the string representation
func (s PublishHealthRequest) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishHealthRequest) GoString() string {
	return s.String()
}

type PublishMetricsInput struct {
	_ struct{} `type:"structure"`

	Metadata *MetricsMetadata `locationName:"metadata" type:"structure"`

	TaskMetrics []*TaskMetric `locationName:"taskMetrics" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp"`
}

// String returns the string representation
func (s PublishMetricsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishMetricsInput) GoString() string {
	return s.String()
}

type PublishMetricsOutput struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s PublishMetricsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishMetricsOutput) GoString() string {
	return s.String()
}

type PublishMetricsRequest struct {
	_ struct{} `type:"structure"`

	Metadata *MetricsMetadata `locationName:"metadata" type:"structure"`

	TaskMetrics []*TaskMetric `locationName:"taskMetrics" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp"`
}

// String returns the string representation
func (s PublishMetricsRequest) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishMetricsRequest) GoString() string {
	return s.String()
}

type ResourceValidationException struct {
	_ struct{} `type:"structure"`

	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ResourceValidationException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ResourceValidationException) GoString() string {
	return s.String()
}

type ServerException struct {
	_ struct{} `type:"structure"`

	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ServerException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ServerException) GoString() string {
	return s.String()
}

type StartTelemetrySessionInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`
}

// String returns the string representation
func (s StartTelemetrySessionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartTelemetrySessionInput) GoString() string {
	return s.String()
}

type StartTelemetrySessionOutput struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s StartTelemetrySessionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartTelemetrySessionOutput) GoString() string {
	return s.String()
}

type StartTelemetrySessionRequest struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`
}

// String returns the string representation
func (s StartTelemetrySessionRequest) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartTelemetrySessionRequest) GoString() string {
	return s.String()
}

type StopTelemetrySessionMessage struct {
	_ struct{} `type:"structure"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s StopTelemetrySessionMessage) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StopTelemetrySessionMessage) GoString() string {
	return s.String()
}

type TaskHealth struct {
	_ struct{} `type:"structure"`

	Containers []*ContainerHealth `locationName:"containers" type:"list"`

	TaskArn *string `locationName:"taskArn" type:"string"`

	TaskDefinitionFamily *string `locationName:"taskDefinitionFamily" type:"string"`

	TaskDefinitionVersion *string `locationName:"taskDefinitionVersion" type:"string"`
}

// String returns the string representation
func (s TaskHealth) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskHealth) GoString() string {
	return s.String()
}

type TaskMetric struct {
	_ struct{} `type:"structure"`

	ContainerMetrics []*ContainerMetric `locationName:"containerMetrics" type:"list"`

	TaskArn *string `locationName:"taskArn" type:"string"`

	TaskDefinitionFamily *string `locationName:"taskDefinitionFamily" type:"string"`

	TaskDefinitionVersion *string `locationName:"taskDefinitionVersion" type:"string"`
}

// String returns the string representation
func (s TaskMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskMetric) GoString() string {
	return s.String()
}
