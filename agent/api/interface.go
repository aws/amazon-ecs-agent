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

package api

import (
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	prometheus "github.com/prometheus/client_model/go"
)

// ECSClient is an interface over the ECSSDK interface which abstracts away some
// details around constructing the request and reading the response down to the
// parts the agent cares about.
// For example, the ever-present 'Cluster' member is abstracted out so that it
// may be configured once and used throughout transparently.
type ECSClient interface {
	// RegisterContainerInstance calculates the appropriate resources, creates
	// the default cluster if necessary, and returns the registered
	// ContainerInstanceARN if successful. Supplying a non-empty container
	// instance ARN allows a container instance to update its registered
	// resources.
	RegisterContainerInstance(existingContainerInstanceArn string,
		attributes []*ecs.Attribute, tags []*ecs.Tag, registrationToken string, platformDevices []*ecs.PlatformDevice,
		outpostARN string) (string, string, error)
	// SubmitTaskStateChange sends a state change and returns an error
	// indicating if it was submitted
	SubmitTaskStateChange(change TaskStateChange) error
	// SubmitContainerStateChange sends a state change and returns an error
	// indicating if it was submitted
	SubmitContainerStateChange(change ContainerStateChange) error
	// SubmitAttachmentStateChange sends an attachment state change and returns an error
	// indicating if it was submitted
	SubmitAttachmentStateChange(change AttachmentStateChange) error
	// DiscoverPollEndpoint takes a ContainerInstanceARN and returns the
	// endpoint at which this Agent should contact ACS
	DiscoverPollEndpoint(containerInstanceArn string) (string, error)
	// DiscoverTelemetryEndpoint takes a ContainerInstanceARN and returns the
	// endpoint at which this Agent should contact Telemetry Service
	DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error)
	// GetResourceTags retrieves the Tags associated with a certain resource
	GetResourceTags(resourceArn string) ([]*ecs.Tag, error)
	// UpdateContainerInstancesState updates the given container Instance ID with
	// the given status. Only valid statuses are ACTIVE and DRAINING.
	UpdateContainerInstancesState(instanceARN, status string) error
}

// ECSSDK is an interface that specifies the subset of the AWS Go SDK's ECS
// client that the Agent uses.  This interface is meant to allow injecting a
// mock for testing.
type ECSSDK interface {
	CreateCluster(*ecs.CreateClusterInput) (*ecs.CreateClusterOutput, error)
	RegisterContainerInstance(*ecs.RegisterContainerInstanceInput) (*ecs.RegisterContainerInstanceOutput, error)
	DiscoverPollEndpoint(*ecs.DiscoverPollEndpointInput) (*ecs.DiscoverPollEndpointOutput, error)
	ListTagsForResource(*ecs.ListTagsForResourceInput) (*ecs.ListTagsForResourceOutput, error)
	UpdateContainerInstancesState(input *ecs.UpdateContainerInstancesStateInput) (*ecs.UpdateContainerInstancesStateOutput, error)
}

// ECSSubmitStateSDK is an interface with customized ecs client that
// implements the SubmitTaskStateChange and SubmitContainerStateChange
type ECSSubmitStateSDK interface {
	SubmitContainerStateChange(*ecs.SubmitContainerStateChangeInput) (*ecs.SubmitContainerStateChangeOutput, error)
	SubmitTaskStateChange(*ecs.SubmitTaskStateChangeInput) (*ecs.SubmitTaskStateChangeOutput, error)
	SubmitAttachmentStateChanges(*ecs.SubmitAttachmentStateChangesInput) (*ecs.SubmitAttachmentStateChangesOutput, error)
}

// AppnetClient is an interface with customized Appnet client that
// implements the GetStats and DrainInboundConnections
type AppnetClient interface {
	GetStats(config serviceconnect.RuntimeConfig) (map[string]*prometheus.MetricFamily, error)
	DrainInboundConnections(config serviceconnect.RuntimeConfig) error
}
