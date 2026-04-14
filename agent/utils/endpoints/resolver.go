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
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/smithy-go/ptr"
)

const (
	// DefaultEndpointResolutionTimeout is the default timeout for endpoint resolution operations
	DefaultEndpointResolutionTimeout = 10 * time.Second
)

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
