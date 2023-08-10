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
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
)

// ECSTaskProtectionSDK is an interface with customized ecs client that
// implements the UpdateTaskProtection and GetTaskProtection
type ECSTaskProtectionSDK interface {
	UpdateTaskProtection(input *ecs.UpdateTaskProtectionInput) (*ecs.UpdateTaskProtectionOutput, error)
	UpdateTaskProtectionWithContext(ctx aws.Context, input *ecs.UpdateTaskProtectionInput,
		opts ...request.Option) (*ecs.UpdateTaskProtectionOutput, error)
	GetTaskProtection(input *ecs.GetTaskProtectionInput) (*ecs.GetTaskProtectionOutput, error)
	GetTaskProtectionWithContext(ctx aws.Context, input *ecs.GetTaskProtectionInput,
		opts ...request.Option) (*ecs.GetTaskProtectionOutput, error)
}

// ECSDiscoverEndpointSDK is an interface with customized ecs client that
// implements the DiscoverPollEndpoint, DiscoverTelemetryEndpoint, and DiscoverServiceConnectEndpoint
type ECSDiscoverEndpointSDK interface {
	DiscoverPollEndpoint(containerInstanceArn string) (string, error)
	DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error)
	DiscoverServiceConnectEndpoint(containerInstanceArn string) (string, error)
}
