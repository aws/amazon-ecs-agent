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

import "time"

type AckPublishMetric struct {
	Message *string `locationName:"message" type:"string"`

	metadataAckPublishMetric `json:"-", xml:"-"`
}

type metadataAckPublishMetric struct {
	SDKShapeTraits bool `type:"structure"`
}

type BadRequestException struct {
	Message *string `locationName:"message" type:"string"`

	metadataBadRequestException `json:"-", xml:"-"`
}

type metadataBadRequestException struct {
	SDKShapeTraits bool `type:"structure"`
}

type CWStatsSet struct {
	Max *float64 `locationName:"max" type:"double"`

	Min *float64 `locationName:"min" type:"double"`

	SampleCount *int64 `locationName:"sampleCount" type:"integer"`

	Sum *float64 `locationName:"sum" type:"double"`

	Unit *string `locationName:"unit" type:"string"`

	metadataCWStatsSet `json:"-", xml:"-"`
}

type metadataCWStatsSet struct {
	SDKShapeTraits bool `type:"structure"`
}

type ContainerMetadata struct {
	Name *string `locationName:"name" type:"string"`

	metadataContainerMetadata `json:"-", xml:"-"`
}

type metadataContainerMetadata struct {
	SDKShapeTraits bool `type:"structure"`
}

type ContainerMetric struct {
	CpuStatsSet *CWStatsSet `locationName:"cpuStatsSet" type:"structure"`

	MemoryStatsSet *CWStatsSet `locationName:"memoryStatsSet" type:"structure"`

	Metadata *ContainerMetadata `locationName:"metadata" type:"structure"`

	metadataContainerMetric `json:"-", xml:"-"`
}

type metadataContainerMetric struct {
	SDKShapeTraits bool `type:"structure"`
}

type HeartbeatMessage struct {
	Healthy *bool `locationName:"healthy" type:"boolean"`

	metadataHeartbeatMessage `json:"-", xml:"-"`
}

type metadataHeartbeatMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

type HeartbeatOutput struct {
	metadataHeartbeatOutput `json:"-", xml:"-"`
}

type metadataHeartbeatOutput struct {
	SDKShapeTraits bool `type:"structure"`
}

type InvalidParameterException struct {
	Message *string `locationName:"message" type:"string"`

	metadataInvalidParameterException `json:"-", xml:"-"`
}

type metadataInvalidParameterException struct {
	SDKShapeTraits bool `type:"structure"`
}

type MetricsMetadata struct {
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	Idle *bool `locationName:"idle" type:"boolean"`

	metadataMetricsMetadata `json:"-", xml:"-"`
}

type metadataMetricsMetadata struct {
	SDKShapeTraits bool `type:"structure"`
}

type PublishMetricsRequest struct {
	Metadata *MetricsMetadata `locationName:"metadata" type:"structure"`

	TaskMetrics []*TaskMetric `locationName:"taskMetrics" type:"list"`

	Timestamp *time.Time `locationName:"timestamp" type:"timestamp" timestampFormat:"unix"`

	metadataPublishMetricsRequest `json:"-", xml:"-"`
}

type metadataPublishMetricsRequest struct {
	SDKShapeTraits bool `type:"structure"`
}

type ResourceValidationException struct {
	Message *string `locationName:"message" type:"string"`

	metadataResourceValidationException `json:"-", xml:"-"`
}

type metadataResourceValidationException struct {
	SDKShapeTraits bool `type:"structure"`
}

type ServerException struct {
	Message *string `locationName:"message" type:"string"`

	metadataServerException `json:"-", xml:"-"`
}

type metadataServerException struct {
	SDKShapeTraits bool `type:"structure"`
}

type StartTelemetrySessionRequest struct {
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	metadataStartTelemetrySessionRequest `json:"-", xml:"-"`
}

type metadataStartTelemetrySessionRequest struct {
	SDKShapeTraits bool `type:"structure"`
}

type StopTelemetrySessionMessage struct {
	Message *string `locationName:"message" type:"string"`

	metadataStopTelemetrySessionMessage `json:"-", xml:"-"`
}

type metadataStopTelemetrySessionMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

type TaskMetric struct {
	ContainerMetrics []*ContainerMetric `locationName:"containerMetrics" type:"list"`

	TaskArn *string `locationName:"taskArn" type:"string"`

	TaskDefinitionFamily *string `locationName:"taskDefinitionFamily" type:"string"`

	TaskDefinitionVersion *string `locationName:"taskDefinitionVersion" type:"string"`

	metadataTaskMetric `json:"-", xml:"-"`
}

type metadataTaskMetric struct {
	SDKShapeTraits bool `type:"structure"`
}
