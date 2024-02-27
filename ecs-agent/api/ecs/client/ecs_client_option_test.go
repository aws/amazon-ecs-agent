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
	"time"

	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/async"
	"github.com/stretchr/testify/assert"
)

const oneSecond = 1 * time.Second

var callFnAndReturnNilFunc = func(fn func() error) error {
	fn()
	return nil
}

func TestWithFIPSDetected(t *testing.T) {
	client := &ecsClient{
		isFIPSDetected: true,
	}
	option := WithFIPSDetected(false)
	option(client)
	assert.False(t, client.isFIPSDetected)
}

func TestWithDiscoverPollEndpointCacheTTL(t *testing.T) {
	client := &ecsClient{
		pollEndpointCache: async.NewTTLCache(&async.TTL{Duration: defaultPollEndpointCacheTTL}),
	}
	option := WithDiscoverPollEndpointCacheTTL(&async.TTL{Duration: oneSecond})
	option(client)
	assert.Equal(t, oneSecond, client.pollEndpointCache.GetTTL().Duration)
}

func TestWithIPv6PortBindingExcluded(t *testing.T) {
	client := &ecsClient{
		shouldExcludeIPv6PortBinding: true,
	}
	option := WithIPv6PortBindingExcluded(false)
	option(client)
	assert.False(t, client.shouldExcludeIPv6PortBinding)
}

func TestWithSASCCustomRetryBackoff(t *testing.T) {
	client := &ecsClient{} // client.sascCustomRetryBackoff is nil by default
	option := WithSASCCustomRetryBackoff(callFnAndReturnNilFunc)
	option(client)
	assert.NotNil(t, client.sascCustomRetryBackoff)
}

func TestWithSTSCAttachmentCustomRetryBackoff(t *testing.T) {
	client := &ecsClient{} // client.stscAttachmentCustomRetryBackoff is nil by default
	option := WithSTSCAttachmentCustomRetryBackoff(callFnAndReturnNilFunc)
	option(client)
	assert.NotNil(t, client.stscAttachmentCustomRetryBackoff)
}

func TestWithDiscoverPollEndpointCache(t *testing.T) {
	client := &ecsClient{
		pollEndpointCache: async.NewTTLCache(&async.TTL{Duration: defaultPollEndpointCacheTTL}),
	}
	newPollEndpointCache := async.NewTTLCache(&async.TTL{Duration: oneSecond})
	option := WithDiscoverPollEndpointCache(newPollEndpointCache)
	option(client)
	assert.Equal(t, newPollEndpointCache, client.pollEndpointCache)
}

func TestWithStandardClient(t *testing.T) {
	client := &ecsClient{
		standardClient: &mock_ecs.MockECSStandardSDK{},
	}
	newStandardClient := mock_ecs.NewMockECSStandardSDK(nil)
	option := WithStandardClient(newStandardClient)
	option(client)
	assert.Equal(t, newStandardClient, client.standardClient)
}

func TestWithSubmitStateChangeClient(t *testing.T) {
	client := &ecsClient{
		submitStateChangeClient: &mock_ecs.MockECSSubmitStateSDK{},
	}
	newSubmitStateChangeClient := mock_ecs.NewMockECSSubmitStateSDK(nil)
	option := WithSubmitStateChangeClient(newSubmitStateChangeClient)
	option(client)
	assert.Equal(t, newSubmitStateChangeClient, client.submitStateChangeClient)
}
