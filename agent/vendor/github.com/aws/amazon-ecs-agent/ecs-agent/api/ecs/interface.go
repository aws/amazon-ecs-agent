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

package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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
		attributes []types.Attribute, tags []types.Tag, registrationToken string, platformDevices []types.PlatformDevice,
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
	// DiscoverServiceConnectEndpoint takes a ContainerInstanceARN and returns the
	// endpoint at which this Agent should contact ServiceConnect
	DiscoverServiceConnectEndpoint(containerInstanceArn string) (string, error)
	// DiscoverSystemLogsEndpoint takes a ContainerInstanceARN and its availability zone
	// and returns the endpoint at which this Agent should send system logs.
	DiscoverSystemLogsEndpoint(containerInstanceArn string, availabilityZone string) (string, error)
	// GetResourceTags retrieves the Tags associated with a certain resource
	GetResourceTags(ctx context.Context, resourceArn string) ([]types.Tag, error)
	// UpdateContainerInstancesState updates the given container Instance ID with
	// the given status. Only valid statuses are ACTIVE and DRAINING.
	UpdateContainerInstancesState(instanceARN string, status types.ContainerInstanceStatus) error
	// GetHostResources retrieves a map that map the resource name to the corresponding resource
	GetHostResources() (map[string]types.Resource, error)
}

// ECSSDK is an interface that specifies the subset of the AWS Go SDK's ECS
// client that the Agent uses.  This interface is meant to allow injecting a
// mock for testing.
type ECSStandardSDK interface {
	CreateCluster(context.Context, *ecs.CreateClusterInput, ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error)
	RegisterContainerInstance(context.Context, *ecs.RegisterContainerInstanceInput, ...func(*ecs.Options)) (*ecs.RegisterContainerInstanceOutput, error)
	DiscoverPollEndpoint(context.Context, *ecs.DiscoverPollEndpointInput, ...func(*ecs.Options)) (*ecs.DiscoverPollEndpointOutput, error)
	ListTagsForResource(context.Context, *ecs.ListTagsForResourceInput, ...func(*ecs.Options)) (*ecs.ListTagsForResourceOutput, error)
	UpdateContainerInstancesState(context.Context, *ecs.UpdateContainerInstancesStateInput, ...func(*ecs.Options)) (*ecs.UpdateContainerInstancesStateOutput, error)
}

// ECSSubmitStateSDK is an interface with customized ecs client that
// implements the SubmitTaskStateChange and SubmitContainerStateChange
type ECSSubmitStateSDK interface {
	SubmitContainerStateChange(context.Context, *ecs.SubmitContainerStateChangeInput, ...func(*ecs.Options)) (*ecs.SubmitContainerStateChangeOutput, error)
	SubmitTaskStateChange(context.Context, *ecs.SubmitTaskStateChangeInput, ...func(*ecs.Options)) (*ecs.SubmitTaskStateChangeOutput, error)
	SubmitAttachmentStateChanges(context.Context, *ecs.SubmitAttachmentStateChangesInput, ...func(*ecs.Options)) (*ecs.SubmitAttachmentStateChangesOutput, error)
}

// ECSTaskProtectionSDK is an interface with customized ecs client that
// implements the UpdateTaskProtection and GetTaskProtection
type ECSTaskProtectionSDK interface {
	UpdateTaskProtection(context.Context, *ecs.UpdateTaskProtectionInput, ...func(*ecs.Options)) (*ecs.UpdateTaskProtectionOutput, error)
	GetTaskProtection(context.Context, *ecs.GetTaskProtectionInput, ...func(*ecs.Options)) (*ecs.GetTaskProtectionOutput, error)
}
