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

package ec2metadata

import "github.com/aws/amazon-ecs-agent/ecs-agent/ec2"

// IsIMDSAvailable checks whether IMDS is reachable on the instance.
// AWS SDK v2 does not provide an equivalent of the v1 Available() method.
// This follows the same approach as v1: querying instance-id via GetMetadata.
// Ref: https://github.com/aws/aws-sdk-go/blob/070853e88d22854d2355c2543d0958a5f76ad407/aws/ec2metadata/api.go#L208
func IsIMDSAvailable(client ec2.EC2MetadataClient) bool {
	_, err := client.GetMetadata("instance-id")
	return err == nil
}
