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

package ec2_test

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
	"github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCreateTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClientSDK := mock_ec2.NewMockClientSDK(ctrl)
	testClient := ec2.NewClientImpl("us-west-2")
	testClient.(*ec2.ClientImpl).SetClientSDK(mockClientSDK)

	createTagsInput := &ec2sdk.CreateTagsInput{}
	createTagsOutput := &ec2sdk.CreateTagsOutput{}

	mockClientSDK.EXPECT().CreateTags(createTagsInput).Return(createTagsOutput, nil)

	res, err := testClient.CreateTags(createTagsInput)
	assert.NoError(t, err)
	assert.Equal(t, createTagsOutput, res)
}

func TestDescribeECSTagsForInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	instanceID := "iid"
	mockClientSDK := mock_ec2.NewMockClientSDK(ctrl)
	testClient := ec2.NewClientImpl("us-west-2")
	testClient.(*ec2.ClientImpl).SetClientSDK(mockClientSDK)

	describeTagsOutput := &ec2sdk.DescribeTagsOutput{
		Tags: []*ec2sdk.TagDescription{
			{
				Key:   aws.String("key"),
				Value: aws.String("value"),
			},
			{
				Key:   aws.String("aws:key"),
				Value: aws.String("aws:value"),
			},
			{
				Key:   aws.String("aWS:key"),
				Value: aws.String("value"),
			},
			{
				Key:   aws.String("key"),
				Value: aws.String("Aws:value"),
			},
		},
	}

	mockClientSDK.EXPECT().DescribeTags(gomock.Any()).Do(func(input *ec2sdk.DescribeTagsInput) {
		assert.Equal(t, len(input.Filters), 2)
		assert.Equal(t, aws.StringValue(input.Filters[0].Values[0]), instanceID)
	}).Return(describeTagsOutput, nil)

	tags, err := testClient.DescribeECSTagsForInstance(instanceID)
	assert.NoError(t, err)
	assert.Equal(t, len(tags), 1)
	assert.Equal(t, aws.StringValue(tags[0].Key), "key")
	assert.Equal(t, aws.StringValue(tags[0].Value), "value")
}
