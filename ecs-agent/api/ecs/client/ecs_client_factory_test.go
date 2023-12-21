//go:build unit
// +build unit

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
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestECSClientFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsProvider := credentials.AnonymousCredentials
	cfgAccessor := newMockConfigAccessor(ctrl, nil)
	ec2MetadataClient := ec2.NewBlackholeEC2MetadataClient()
	agentVersion := "0.0.0"

	clientFactory, ok := NewECSClientFactory(credentialsProvider, cfgAccessor, ec2MetadataClient,
		agentVersion).(*ecsClientFactory)
	assert.True(t, ok)
	assert.Equal(t, credentialsProvider, clientFactory.credentialsProvider)
	assert.Equal(t, cfgAccessor, clientFactory.configAccessor)
	assert.Equal(t, ec2MetadataClient, clientFactory.ec2MetadataClient)
	assert.Equal(t, agentVersion, clientFactory.agentVersion)

	client, err := clientFactory.NewClient()
	assert.NoError(t, err)
	c, ok := client.(*ecsClient)
	assert.True(t, ok)
	assert.Equal(t, credentialsProvider, c.credentialsProvider)
	assert.Equal(t, cfgAccessor, c.configAccessor)
	assert.Equal(t, ec2MetadataClient, c.ec2metadata)

	assert.Equal(t, credentialsProvider, clientFactory.GetCredentials())
}
