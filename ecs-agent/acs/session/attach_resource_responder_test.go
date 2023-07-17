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

package session

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
)

const (
	testAttachmentArn = "arn:aws:ecs:us-west-2:123456789012:ephemeral-storage/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	testClusterArn    = "arn:aws:ecs:us-west-2:123456789012:cluster/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
)

var (
	testAttachmentProperties = []*ecsacs.AttachmentProperty{
		{
			Name:  aws.String(resource.FargateResourceIdName),
			Value: aws.String("name1"),
		},
		{
			Name:  aws.String(resource.VolumeIdName),
			Value: aws.String("id1"),
		},
		{
			Name:  aws.String(resource.VolumeSizeInGiBName),
			Value: aws.String("size1"),
		},
		{
			Name:  aws.String(resource.RequestedSizeName),
			Value: aws.String("size2"),
		},
		{
			Name:  aws.String(resource.ResourceTypeName),
			Value: aws.String(resource.EphemeralStorage),
		},
		{
			Name:  aws.String(resource.DeviceName),
			Value: aws.String("device1"),
		},
	}
	testAttachment = &ecsacs.Attachment{
		AttachmentArn:        aws.String(testAttachmentArn),
		AttachmentProperties: testAttachmentProperties,
	}
	testConfirmAttachmentMessage = &ecsacs.ConfirmAttachmentMessage{
		Attachment:           testAttachment,
		MessageId:            aws.String(testconst.MessageID),
		ClusterArn:           aws.String(testClusterArn),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		TaskArn:              aws.String(testconst.TaskARN),
		WaitTimeoutMs:        aws.Int64(testconst.WaitTimeoutMillis),
	}
)

func TestValidateAttachResourceMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	_, err := validateAttachResourceMessage(nil)

	require.Error(t, err)

	confirmAttachmentMessageCopy := *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.Attachment = nil

	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)

	require.Error(t, err)

	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.MessageId = aws.String("")

	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)

	require.Error(t, err)

	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.ClusterArn = aws.String("")

	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)

	require.Error(t, err)

	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.ContainerInstanceArn = aws.String("")

	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)

	require.Error(t, err)
}

func TestValidateAttachmentAndReturnProperties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	confirmAttachmentMessageCopy := *testConfirmAttachmentMessage

	confirmAttachmentMessageCopy.Attachment.AttachmentArn = aws.String("incorrectArn")
	_, err := validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
	require.Error(t, err)
	confirmAttachmentMessageCopy.Attachment.AttachmentArn = aws.String(testAttachmentArn)

	for _, property := range confirmAttachmentMessageCopy.Attachment.AttachmentProperties {
		t.Run(property.String(), func(t *testing.T) {
			originalPropertyName := property.Name
			property.Name = aws.String("")
			_, err := validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
			require.Error(t, err)
			property.Name = originalPropertyName

			originalPropertyValue := property.Value
			property.Value = aws.String("")
			_, err = validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
			require.Error(t, err)
			property.Value = originalPropertyValue

			if aws.StringValue(originalPropertyName) == resource.ResourceTypeName {
				property.Name = aws.String("not resourceType")
				_, err = validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
				require.Error(t, err)
				property.Name = originalPropertyName
			}
		})
	}
}
