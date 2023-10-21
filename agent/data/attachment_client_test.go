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

package data

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/stretchr/testify/assert"
)

const (
	testAttachmentArn  = "arn:aws:ecs:us-west-2:167933679560:attachment/test-arn"
	testAttachmentArn2 = "arn:aws:ecs:us-west-2:167933679560:attachment/test-arn2"
	testAttachmentArn3 = "arn:aws:ecs:us-west-2:123456789012:volume/test-arn3"
	testAttachmentArn4 = "arn:aws:ecs:us-west-2:123456789012:volume/test-arn4"
)

func TestManageENIAttachments(t *testing.T) {
	testClient := newTestClient(t)

	testEniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    testAttachmentArn,
			AttachStatusSent: false,
		},
	}

	assert.NoError(t, testClient.SaveENIAttachment(testEniAttachment))
	testEniAttachment.SetSentStatus()
	assert.NoError(t, testClient.SaveENIAttachment(testEniAttachment))
	res, err := testClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, true, res[0].AttachStatusSent)
	assert.Equal(t, testAttachmentArn, res[0].AttachmentARN)

	testEniAttachment2 := &ni.ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    testAttachmentArn2,
			AttachStatusSent: true,
		},
	}

	assert.NoError(t, testClient.SaveENIAttachment(testEniAttachment2))
	res, err = testClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 2)

	assert.NoError(t, testClient.DeleteENIAttachment("test-arn"))
	assert.NoError(t, testClient.DeleteENIAttachment("test-arn2"))
	res, err = testClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestManageResourceAttachments(t *testing.T) {
	testClient := newTestClient(t)

	testResAttachment := &resource.ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    testAttachmentArn3,
			AttachStatusSent: false,
		},
	}

	assert.NoError(t, testClient.SaveResourceAttachment(testResAttachment))
	testResAttachment.SetSentStatus()
	assert.NoError(t, testClient.SaveResourceAttachment(testResAttachment))
	res, err := testClient.GetResourceAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, true, res[0].AttachStatusSent)
	assert.Equal(t, testAttachmentArn3, res[0].AttachmentARN)

	testResAttachment2 := &resource.ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    testAttachmentArn4,
			AttachStatusSent: true,
		},
	}

	assert.NoError(t, testClient.SaveResourceAttachment(testResAttachment2))
	res, err = testClient.GetResourceAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 2)

	assert.NoError(t, testClient.DeleteResourceAttachment("test-arn3"))
	assert.NoError(t, testClient.DeleteResourceAttachment("test-arn4"))
	res, err = testClient.GetResourceAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestSaveENIAttachmentInvalidID(t *testing.T) {
	testClient := newTestClient(t)

	testEniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    "invalid-arn",
			AttachStatusSent: false,
		},
	}

	assert.Error(t, testClient.SaveENIAttachment(testEniAttachment))
}

func TestSaveResAttachmentInvalidID(t *testing.T) {
	testClient := newTestClient(t)

	testResAttachment := &resource.ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:    "invalid-arn",
			AttachStatusSent: false,
		},
	}

	assert.Error(t, testClient.SaveResourceAttachment(testResAttachment))
}
