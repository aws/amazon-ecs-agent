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
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/credentials/instancecreds"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/cihub/seelog"
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
	DescribeECSTagsForInstance(instanceID string) ([]*ecs.Tag, error)
}

type ClientSDK interface {
	CreateTags(input *ec2sdk.CreateTagsInput) (*ec2sdk.CreateTagsOutput, error)
	DescribeTags(input *ec2sdk.DescribeTagsInput) (*ec2sdk.DescribeTagsOutput, error)
}

type ClientImpl struct {
	client ClientSDK
}

func NewClientImpl(awsRegion string) Client {
	ec2Config := aws.NewConfig().WithMaxRetries(clientRetriesNum)
	ec2Config.Region = aws.String(awsRegion)
	ec2Config.Credentials = instancecreds.GetCredentials()
	client := ec2sdk.New(session.New(), ec2Config)
	return &ClientImpl{
		client: client,
	}
}

// SetSDK overrides the SDK to the given one. This is useful for injecting a
// test implementation
func (c *ClientImpl) SetClientSDK(sdk ClientSDK) {
	c.client = sdk
}

// DescribeECSTagsForInstance calls DescribeTags API to get the EC2 tags of the
// instance id, and return it back as ECS tags
func (c *ClientImpl) DescribeECSTagsForInstance(instanceID string) ([]*ecs.Tag, error) {
	describeTagsInput := ec2sdk.DescribeTagsInput{
		Filters: []*ec2sdk.Filter{
			{
				Name:   aws.String(ResourceIDFilterName),
				Values: []*string{aws.String(instanceID)},
			},
			{
				Name:   aws.String(ResourceTypeFilterName),
				Values: []*string{aws.String(ResourceTypeFilterValueInstance)},
			},
		},
		MaxResults: aws.Int64(InstanceMaxTagsNum),
	}
	res, err := c.client.DescribeTags(&describeTagsInput)

	if err != nil {
		seelog.Criticalf("Error calling DescribeTags API: %v", err)
		return nil, err
	}

	var tags []*ecs.Tag
	// Convert ec2 tags to ecs tags
	for _, ec2Tag := range res.Tags {
		// Filter out all tags "aws:" prefix
		if !strings.HasPrefix(strings.ToLower(aws.StringValue(ec2Tag.Key)), awsTagPrefix) &&
			!strings.HasPrefix(strings.ToLower(aws.StringValue(ec2Tag.Value)), awsTagPrefix) {
			tags = append(tags, &ecs.Tag{
				Key:   ec2Tag.Key,
				Value: ec2Tag.Value,
			})
		}
	}
	return tags, nil
}

func (c *ClientImpl) CreateTags(input *ec2sdk.CreateTagsInput) (*ec2sdk.CreateTagsOutput, error) {
	return c.client.CreateTags(input)
}
