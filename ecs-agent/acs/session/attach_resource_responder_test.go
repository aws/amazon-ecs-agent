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
	"sync"
	"testing"

	mock_session "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
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
	testAttachmentPropertiesForEBSAttach = []*ecsacs.AttachmentProperty{
		{
			Name:  aws.String(resource.SourceVolumeHostPathKey),
			Value: aws.String("taskarn-vol-id"),
		},
		{
			Name:  aws.String(resource.VolumeIdKey),
			Value: aws.String("id1"),
		},
		{
			Name:  aws.String(resource.VolumeSizeGibKey),
			Value: aws.String("size1"),
		},
		{
			Name:  aws.String(resource.VolumeNameKey),
			Value: aws.String("name"),
		},
		{
			Name:  aws.String(resource.DeviceNameKey),
			Value: aws.String("device1"),
		},
	}

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

	// Verify the Attachment is required.
	confirmAttachmentMessageCopy := *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.Attachment = nil
	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)
	require.Error(t, err)

	// Verify the MessageId is required.
	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.MessageId = aws.String("")
	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)
	require.Error(t, err)

	// Verify the ClusterArn is required.
	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.ClusterArn = aws.String("")
	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)
	require.Error(t, err)

	// Verify the ContainerInstanceArn is required.
	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.ContainerInstanceArn = aws.String("")
	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)
	require.Error(t, err)

	// Verify the AttachmentArn is required and uses correct format.
	confirmAttachmentMessageCopy = *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.Attachment.AttachmentArn = aws.String("incorrectArn")
	_, err = validateAttachResourceMessage(&confirmAttachmentMessageCopy)
	require.Error(t, err)
}

func TestValidateAttachmentAndReturnProperties(t *testing.T) {
	t.Run("with no attachment type", testValidateAttachmentAndReturnPropertiesWithoutAttachmentType)
	t.Run("with attachment type", testValidateAttachmentAndReturnPropertiesWithAttachmentType)
}

// testValidateAttachmentAndReturnPropertiesWithoutAttachmentType verifies all required properties for either
// resource type: "EphemeralStorage" or "ElasticBlockStorage".
func testValidateAttachmentAndReturnPropertiesWithoutAttachmentType(t *testing.T) {
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

// testValidateAttachmentAndReturnPropertiesWithAttachmentType verifies all required properties for attachment
// type: "amazonebs".
func testValidateAttachmentAndReturnPropertiesWithAttachmentType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Verify the AttachmentArn is required and uses the correct format.
	confirmAttachmentMessageCopy := *testConfirmAttachmentMessage
	confirmAttachmentMessageCopy.Attachment.AttachmentArn = aws.String("incorrectArn")
	_, err := validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
	require.Error(t, err)
	confirmAttachmentMessageCopy.Attachment.AttachmentArn = aws.String(testAttachmentArn)

	// Verify the property name & property value must be non-empty.
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
		})
	}

	// Reset attachment to be a good one with invalid attachment type.
	confirmAttachmentMessageCopy.Attachment.AttachmentType = aws.String("invalid-type")
	_, err = validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
	require.NoError(t, err)

	// Reset attachment to be a good one with valid attachment type.
	confirmAttachmentMessageCopy.Attachment.AttachmentType = aws.String("amazonebs")
	confirmAttachmentMessageCopy.Attachment.AttachmentProperties = testAttachmentPropertiesForEBSAttach

	// Verify all required properties for the attachment for EBS attach are present.
	requiredProperties := []string{
		"volumeId",
		"volumeSizeGib",
		"deviceName",
		"sourceVolumeHostPath",
		"volumeName",
	}
	for _, requiredProperty := range requiredProperties {
		verified := false
		for _, property := range confirmAttachmentMessageCopy.Attachment.AttachmentProperties {
			if requiredProperty == aws.StringValue(property.Name) {
				originalPropertyName := property.Name
				property.Name = aws.String("")
				_, err := validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
				require.Error(t, err)
				property.Name = originalPropertyName

				verified = true
				break
			}
		}
		require.True(t, verified, "Missing required property: %s", requiredProperty)
	}
}

// TestResourceAckHappyPath tests the happy path for a typical ConfirmAttachmentMessage and confirms expected
// ACK request is made
func TestResourceAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	confirmAttachmentMessageCopy := *testConfirmAttachmentMessage

	ackSent := make(chan *ecsacs.AckRequest)

	_, err := validateAttachmentAndReturnProperties(&confirmAttachmentMessageCopy)
	require.NoError(t, err)

	// WaitGroup is necessary to wait for function to be called in separate goroutine before exiting the test.
	wg := sync.WaitGroup{}
	wg.Add(1)

	mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockEntry.EXPECT().Done(err)
	mockMetricsFactory.EXPECT().New(metrics.ResourceValidationMetricName).Return(mockEntry)
	mockResourceHandler := mock_session.NewMockResourceHandler(ctrl)
	mockResourceHandler.EXPECT().
		HandleResourceAttachment(gomock.Any()).
		Do(func(arg0 interface{}) {
			defer wg.Done() // decrement WaitGroup counter now that HandleResourceAttachment function has been called
		})

	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}
	testResourceResponder := NewAttachResourceResponder(
		mockResourceHandler,
		mockMetricsFactory,
		testResponseSender)

	handleAttachMessage := testResourceResponder.HandlerFunc().(func(*ecsacs.ConfirmAttachmentMessage))
	go handleAttachMessage(&confirmAttachmentMessageCopy)

	attachResourceAckSent := <-ackSent
	wg.Wait()
	require.Equal(t, aws.StringValue(attachResourceAckSent.MessageId), testconst.MessageID)
}
