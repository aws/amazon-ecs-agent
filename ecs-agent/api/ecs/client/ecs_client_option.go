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
	"github.com/aws/amazon-ecs-agent/ecs-agent/async"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
)

// ECSClientOption allows for configuration of an ecsClient.
type ECSClientOption func(*ecsClient)

// WithFIPSDetected is an ECSClientOption that configures the
// ecsClient.isFIPSDetected with the value passed as a parameter.
func WithFIPSDetected(val bool) ECSClientOption {
	return func(client *ecsClient) {
		client.isFIPSDetected = val
	}
}

// WithDualStackEnabled is an ECSClientOption that configures the
// ecsClient.isDualStackEnabled with the value passed as a parameter.
func WithDualStackEnabled(val bool) ECSClientOption {
	return func(client *ecsClient) {
		client.isDualStackEnabled = val
	}
}

// WithDiscoverPollEndpointCacheTTL is an ECSClientOption that configures the
// ecsClient.pollEndpointCache.ttl with the value passed as a parameter.
func WithDiscoverPollEndpointCacheTTL(t *async.TTL) ECSClientOption {
	return func(client *ecsClient) {
		client.pollEndpointCache.SetTTL(t)
	}
}

// WithIPv6PortBindingExcluded is an ECSClientOption that configures the
// ecsClient.shouldExcludeIPv6PortBinding with the value passed as a parameter.
func WithIPv6PortBindingExcluded(val bool) ECSClientOption {
	return func(client *ecsClient) {
		client.shouldExcludeIPv6PortBinding = val
	}
}

// WithIPv4PortBindingExcluded is an ECSClientOption that configures the
// ecsClient.shouldExcludeIPv4PortBinding with the value passed as a parameter.
func WithIPv4PortBindingExcluded(val bool) ECSClientOption {
	return func(client *ecsClient) {
		client.shouldExcludeIPv4PortBinding = val
	}
}

// WithSASCCustomRetryBackoff is an ECSClientOption that configures the
// ecsClient.sascCustomRetryBackoff with the value passed as a parameter.
func WithSASCCustomRetryBackoff(f func(func() error) error) ECSClientOption {
	return func(client *ecsClient) {
		client.sascCustomRetryBackoff = f
	}
}

// WithSTSCAttachmentCustomRetryBackoff is an ECSClientOption that configures the
// ecsClient.stscAttachmentCustomRetryBackoff with the value passed as a parameter.
func WithSTSCAttachmentCustomRetryBackoff(f func(func() error) error) ECSClientOption {
	return func(client *ecsClient) {
		client.stscAttachmentCustomRetryBackoff = f
	}
}

// WithRCICustomRetryBackoff is an ECSClientOption that configures the
// ecsClient.rciRetryBackoff with the value passed as a parameter.
func WithRCICustomRetryBackoff(backoff *retry.ExponentialBackoff) ECSClientOption {
	return func(client *ecsClient) {
		client.rciRetryBackoff = backoff
	}
}

// WithDiscoverPollEndpointCache is an ECSClientOption that configures the
// ecsClient.pollEndpointCache with the value passed as a parameter.
// This is especially useful for injecting a test implementation.
func WithDiscoverPollEndpointCache(c async.TTLCache) ECSClientOption {
	return func(client *ecsClient) {
		client.pollEndpointCache = c
	}
}

// WithStandardClient is an ECSClientOption that configures the
// ecsClient.standardClient with the value passed as a parameter.
// This is especially useful for injecting a test implementation.
func WithStandardClient(s ecs.ECSStandardSDK) ECSClientOption {
	return func(client *ecsClient) {
		client.standardClient = s
	}
}

// WithSubmitStateChangeClient is an ECSClientOption that configures the
// ecsClient.submitStateChangeClient with the value passed as a parameter.
// This is especially useful for injecting a test implementation.
func WithSubmitStateChangeClient(s ecs.ECSSubmitStateSDK) ECSClientOption {
	return func(client *ecsClient) {
		client.submitStateChangeClient = s
	}
}

// WithMetricsFactory is an ECSClientOption that configures
// ecsClient.metricsFactory with the value passed as a parameter.
// This is especially useful for emitting metrics in the ECS Client
func WithMetricsFactory(metricsFactory metrics.EntryFactory) ECSClientOption {
	return func(client *ecsClient) {
		client.metricsFactory = metricsFactory
	}
}

// WithAvailableMemoryProvider is an ECSClientOption that configures a
// custom strategy for returning the default amount of available memory
// on a container instance in MiB.
func WithAvailableMemoryProvider(availableMemoryProvider func() int32) ECSClientOption {
	return func(client *ecsClient) {
		client.availableMemoryProvider = availableMemoryProvider
	}
}
