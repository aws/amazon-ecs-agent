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

package ecsclient

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/config"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

// ECSClientFactory interface can be used to create new ECS clients.
// It wraps the internal ecsClientFactory and can help ease writing unit test code for consumers.
type ECSClientFactory interface {
	// NewClient creates a new ECS client.
	NewClient() (ecs.ECSClient, error)
	// GetCredentials returns the credentials chain used by the ECS client.
	GetCredentials() *credentials.Credentials
}

type ecsClientFactory struct {
	credentialsProvider *credentials.Credentials
	configAccessor      config.AgentConfigAccessor
	ec2MetadataClient   ec2.EC2MetadataClient
	agentVersion        string
	options             []ECSClientOption
}

func NewECSClientFactory(
	credentialsProvider *credentials.Credentials,
	configAccessor config.AgentConfigAccessor,
	ec2MetadataClient ec2.EC2MetadataClient,
	agentVersion string,
	options ...ECSClientOption) ECSClientFactory {
	return &ecsClientFactory{
		credentialsProvider: credentialsProvider,
		configAccessor:      configAccessor,
		ec2MetadataClient:   ec2MetadataClient,
		agentVersion:        agentVersion,
		options:             options,
	}
}

func (f *ecsClientFactory) NewClient() (ecs.ECSClient, error) {
	return NewECSClient(f.credentialsProvider, f.configAccessor, f.ec2MetadataClient, f.agentVersion, f.options...)
}

func (f *ecsClientFactory) GetCredentials() *credentials.Credentials {
	return f.credentialsProvider
}
