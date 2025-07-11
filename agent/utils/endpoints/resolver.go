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

// Package endpoints provides functions for resolving AWS service endpoints,
// particularly for dual-stack (IPv4/IPv6) environments.
package endpoints

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/ptr"
)

const (
	// DefaultEndpointResolutionTimeout is the default timeout for endpoint resolution operations
	DefaultEndpointResolutionTimeout = 10 * time.Second
)

// resolveDualStackEndpoint is a helper function that constructs dual stack endpoints
// specifically for ssmmessages and ec2messages internal AWS services using a predefined format.
// This is a temporary solution until amazon-ssm-agent can be configured to support
// dual stack endpoints. This function should only be used for these two specific services
// and is not intended as a general endpoint resolver.
func resolveDualStackEndpoint(serviceName, region string) (string, error) {
	if region == "" {
		return "", fmt.Errorf("region is required to resolve %s dual stack endpoint", serviceName)
	}

	// Determine the domain suffix based on the region
	// For China regions, use api.amazonwebservices.com.cn
	// For AWS and US Gov Cloud regions, use api.aws
	domainSuffix := "api.aws"
	if strings.HasPrefix(region, "cn-") {
		domainSuffix = "api.amazonwebservices.com.cn"
	}

	// Construct the endpoint using the predefined format
	endpoint := fmt.Sprintf("https://%s.%s.%s", serviceName, region, domainSuffix)

	return endpoint, nil
}

// ResolveSSMMessagesDualStackEndpoint resolves the dual stack endpoint for SSM Messages service
// using a predefined format. This is a temporary solution until amazon-ssm-agent can be
// configured to support dual stack endpoints. This function will be removed once
// amazon-ssm-agent supports configurable dual stack endpoints.
func ResolveSSMMessagesDualStackEndpoint(region string) (string, error) {
	return resolveDualStackEndpoint("ssmmessages", region)
}

// ResolveEC2MessagesDualStackEndpoint resolves the dual stack endpoint for EC2 Messages service
// using a predefined format. This is a temporary solution until amazon-ssm-agent can be
// configured to support dual stack endpoints. This function will be removed once
// amazon-ssm-agent supports configurable dual stack endpoints.
func ResolveEC2MessagesDualStackEndpoint(region string) (string, error) {
	return resolveDualStackEndpoint("ec2messages", region)
}

// ResolveS3Endpoint resolves the endpoint for S3 service
// using the AWS SDK to ensure proper handling of different partitions
func ResolveS3Endpoint(region string, useDualStack bool) (string, error) {
	if region == "" {
		return "", fmt.Errorf("region is required to resolve S3 endpoint")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultEndpointResolutionTimeout)
	defer cancel()

	// Use the S3 endpoint resolver V2
	endpoint, err := s3.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, s3.EndpointParameters{
		Region:       ptr.String(region),
		UseDualStack: ptr.Bool(useDualStack),
	})
	if err != nil {
		return "", fmt.Errorf("failed to resolve S3 endpoint for region '%s': %w", region, err)
	}

	return endpoint.URI.String(), nil
}

// ResolveKMSEndpoint resolves the endpoint for KMS service
// using the AWS SDK to ensure proper handling of different partitions
func ResolveKMSEndpoint(region string, useDualStack bool) (string, error) {
	if region == "" {
		return "", fmt.Errorf("region is required to resolve KMS endpoint")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultEndpointResolutionTimeout)
	defer cancel()

	// Use the KMS endpoint resolver V2
	endpoint, err := kms.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, kms.EndpointParameters{
		Region:       ptr.String(region),
		UseDualStack: ptr.Bool(useDualStack),
	})
	if err != nil {
		return "", fmt.Errorf("failed to resolve KMS endpoint for region '%s': %w", region, err)
	}

	return endpoint.URI.String(), nil
}

// ResolveCloudWatchLogsEndpoint resolves the endpoint for CloudWatch Logs service
// using the AWS SDK to ensure proper handling of different partitions
func ResolveCloudWatchLogsEndpoint(region string, useDualStack bool) (string, error) {
	if region == "" {
		return "", fmt.Errorf("region is required to resolve CloudWatch Logs endpoint")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultEndpointResolutionTimeout)
	defer cancel()

	// Use the CloudWatch Logs endpoint resolver V2
	endpoint, err := cloudwatchlogs.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx,
		cloudwatchlogs.EndpointParameters{
			UseDualStack: ptr.Bool(useDualStack),
			Region:       ptr.String(region),
		})
	if err != nil {
		return "", fmt.Errorf("failed to resolve CloudWatch Logs endpoint for region '%s': %w", region, err)
	}

	return endpoint.URI.String(), nil
}

// ResolveSSMEndpoint resolves the endpoint for SSM service
// using the AWS SDK to ensure proper handling of different partitions
func ResolveSSMEndpoint(region string, useDualStack bool) (string, error) {
	if region == "" {
		return "", fmt.Errorf("region is required to resolve SSM endpoint")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultEndpointResolutionTimeout)
	defer cancel()

	// Use the SSM endpoint resolver V2
	endpoint, err := ssm.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, ssm.EndpointParameters{
		Region:       ptr.String(region),
		UseDualStack: ptr.Bool(useDualStack),
	})
	if err != nil {
		return "", fmt.Errorf("failed to resolve SSM endpoint for region '%s': %w", region, err)
	}

	return endpoint.URI.String(), nil
}
