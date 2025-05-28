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

package ec2

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/providers"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const (
	clientRetriesNum = 5

	// InstanceMaxTagsNum is the maximum number of tags that an instance can have.
	InstanceMaxTagsNum = 50

	ResourceIDFilterName            = "resource-id"
	ResourceTypeFilterName          = "resource-type"
	ResourceTypeFilterValueInstance = "instance"
	awsTagPrefix                    = "aws:"
)

type Client interface {
	CreateTags(input *ec2sdk.CreateTagsInput) (*ec2sdk.CreateTagsOutput, error)
	DescribeECSTagsForInstance(instanceID string) ([]ecstypes.Tag, error)
}

type ClientSDK interface {
	CreateTags(ctx context.Context, input *ec2sdk.CreateTagsInput, optsFns ...func(*ec2sdk.Options)) (*ec2sdk.CreateTagsOutput, error)
	DescribeTags(ctx context.Context, input *ec2sdk.DescribeTagsInput, optsFns ...func(*ec2sdk.Options)) (*ec2sdk.DescribeTagsOutput, error)
}

type ClientImpl struct {
	client ClientSDK
}

func NewClientImpl(
	awsRegion string,
	dualStackEndpointState aws.DualStackEndpointState,
) (Client, error) {
	credentialsProvider := providers.NewInstanceCredentialsCache(
		false,
		providers.NewRotatingSharedCredentialsProviderV2(),
		nil,
	)
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(credentialsProvider),
		config.WithRetryMaxAttempts(clientRetriesNum),
		config.WithUseDualStackEndpoint(dualStackEndpointState),
	)
	if err != nil {
		return nil, err
	}

	return &ClientImpl{
		client: ec2sdk.NewFromConfig(cfg),
	}, nil
}

// SetSDK overrides the SDK to the given one. This is useful for injecting a
// test implementation
func (c *ClientImpl) SetClientSDK(sdk ClientSDK) {
	c.client = sdk
}

// DescribeECSTagsForInstance calls DescribeTags API to get the EC2 tags of the
// instance id, and return it back as ECS tags
func (c *ClientImpl) DescribeECSTagsForInstance(instanceID string) ([]ecstypes.Tag, error) {
	describeTagsInput := ec2sdk.DescribeTagsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String(ResourceIDFilterName),
				Values: []string{instanceID},
			},
			{
				Name:   aws.String(ResourceTypeFilterName),
				Values: []string{ResourceTypeFilterValueInstance},
			},
		},
		MaxResults: aws.Int32(InstanceMaxTagsNum),
	}
	res, err := c.client.DescribeTags(context.TODO(), &describeTagsInput)

	if err != nil {
		logger.Critical(fmt.Sprintf("Error calling DescribeTags API: %v", err))
		return nil, err
	}

	var tags []ecstypes.Tag
	// Convert ec2 tags to ecs tags
	for _, ec2Tag := range res.Tags {
		// Filter out all tags "aws:" prefix
		if !strings.HasPrefix(strings.ToLower(aws.ToString(ec2Tag.Key)), awsTagPrefix) &&
			!strings.HasPrefix(strings.ToLower(aws.ToString(ec2Tag.Value)), awsTagPrefix) {
			tags = append(tags, ecstypes.Tag{
				Key:   ec2Tag.Key,
				Value: ec2Tag.Value,
			})
		}
	}
	return tags, nil
}

func (c *ClientImpl) CreateTags(input *ec2sdk.CreateTagsInput) (*ec2sdk.CreateTagsOutput, error) {
	return c.client.CreateTags(context.TODO(), input)
}
