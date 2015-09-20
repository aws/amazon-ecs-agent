// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

type AckPublishMetric struct {
	Message *string `locationName:"message" type:"string"`

	metadataAckPublishMetric `json:"-" xml:"-"`
}

type metadataAckPublishMetric struct {
	SDKShapeTraits bool `type:"structure"`
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
	Message *string `locationName:"message" type:"string"`

	metadataBadRequestException `json:"-" xml:"-"`
}

type metadataBadRequestException struct {
	SDKShapeTraits bool `type:"structure"`
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
	Max *float64 `locationName:"max" type:"double"`

	Min *float64 `locationName:"min" type:"double"`

	SampleCount *int64 `locationName:"sampleCount" type:"integer"`

	Sum *float64 `locationName:"sum" type:"double"`

	metadataCWStatsSet `json:"-" xml:"-"`
}

type metadataCWStatsSet struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s CWStatsSet) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CWStatsSet) GoString() string {
	return s.String()
}

type ContainerMetric struct {
	CpuStatsSet *CWStatsSet `locationName:"cpuStatsSet" type:"structure"`

	MemoryStatsSet *CWStatsSet `locationName:"memoryStatsSet" type:"structure"`

	metadataContainerMetric `json:"-" xml:"-"`
}

type metadataContainerMetric struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s ContainerMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerMetric) GoString() string {
	return s.String()
}

type HeartbeatMessage struct {
	Healthy *bool `locationName:"healthy" type:"boolean"`

	metadataHeartbeatMessage `json:"-" xml:"-"`
}

type metadataHeartbeatMessage struct {
	SDKShapeTraits bool `type:"structure"`
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
	metadataHeartbeatOutput `json:"-" xml:"-"`
}

type metadataHeartbeatOutput struct {
	SDKShapeTraits bool `type:"structure"`
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
	Message *string `locationName:"message" type:"string"`

	metadataInvalidParameterException `json:"-" xml:"-"`
}

type metadataInvalidParameterException struct {
	SDKShapeTraits bool `type:"structure"`
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
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	Fin *bool `locationName:"fin" type:"boolean"`

	Idle *bool `locationName:"idle" type:"boolean"`

	MessageId *string `locationName:"messageId" type:"string"`

	metadataMetricsMetadata `json:"-" xml:"-"`
}

type metadataMetricsMetadata struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s MetricsMetadata) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s MetricsMetadata) GoString() string {
	return s.String()
}

type PublishMetricsRequest struct {
	Metadata *MetricsMetadata `locationName:"metadata" type:"structure"`

	TaskMetrics []*TaskMetric `locationName:"taskMetrics" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp" timestampFormat:"unix"`

	metadataPublishMetricsRequest `json:"-" xml:"-"`
}

type metadataPublishMetricsRequest struct {
	SDKShapeTraits bool `type:"structure"`
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
	Message *string `locationName:"message" type:"string"`

	metadataResourceValidationException `json:"-" xml:"-"`
}

type metadataResourceValidationException struct {
	SDKShapeTraits bool `type:"structure"`
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
	Message *string `locationName:"message" type:"string"`

	metadataServerException `json:"-" xml:"-"`
}

type metadataServerException struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s ServerException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ServerException) GoString() string {
	return s.String()
}

type StartTelemetrySessionRequest struct {
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	metadataStartTelemetrySessionRequest `json:"-" xml:"-"`
}

type metadataStartTelemetrySessionRequest struct {
	SDKShapeTraits bool `type:"structure"`
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
	Message *string `locationName:"message" type:"string"`

	metadataStopTelemetrySessionMessage `json:"-" xml:"-"`
}

type metadataStopTelemetrySessionMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s StopTelemetrySessionMessage) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StopTelemetrySessionMessage) GoString() string {
	return s.String()
}

type TaskMetric struct {
	ContainerMetrics []*ContainerMetric `locationName:"containerMetrics" type:"list"`

	TaskArn *string `locationName:"taskArn" type:"string"`

	TaskDefinitionFamily *string `locationName:"taskDefinitionFamily" type:"string"`

	TaskDefinitionVersion *string `locationName:"taskDefinitionVersion" type:"string"`

	metadataTaskMetric `json:"-" xml:"-"`
}

type metadataTaskMetric struct {
	SDKShapeTraits bool `type:"structure"`
}

// String returns the string representation
func (s TaskMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskMetric) GoString() string {
	return s.String()
}
