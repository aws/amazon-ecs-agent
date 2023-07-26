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

package resource

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"

	"github.com/stretchr/testify/assert"
)

const (
	taskARN       = "t1"
	attachmentARN = "att1"
	attachSent    = true
)

var (
	testAttachmentProperties = map[string]string{
		ResourceTypeName:    ElasticBlockStorage,
		RequestedSizeName:   "5",
		VolumeSizeInGiBName: "7",
		DeviceName:          "/dev/nvme0n0",
		VolumeIdName:        "vol-123",
		FileSystemTypeName:  "testXFS",
	}
)

func TestMarshalUnmarshal(t *testing.T) {
	expiresAt := time.Now()
	attachment := &ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           status.AttachmentNone,
			ExpiresAt:        expiresAt,
		},
		AttachmentProperties: testAttachmentProperties,
	}
	bytes, err := json.Marshal(attachment)
	assert.NoError(t, err)
	var unmarshalledAttachment ResourceAttachment
	err = json.Unmarshal(bytes, &unmarshalledAttachment)
	assert.NoError(t, err)
	assert.Equal(t, attachment.TaskARN, unmarshalledAttachment.TaskARN)
	assert.Equal(t, attachment.AttachmentARN, unmarshalledAttachment.AttachmentARN)
	assert.Equal(t, attachment.AttachStatusSent, unmarshalledAttachment.AttachStatusSent)
	assert.Equal(t, attachment.Status, unmarshalledAttachment.Status)

	assert.Equal(t, attachment.AttachmentProperties[ResourceTypeName], unmarshalledAttachment.AttachmentProperties[ResourceTypeName])
	assert.Equal(t, attachment.AttachmentProperties[RequestedSizeName], unmarshalledAttachment.AttachmentProperties[RequestedSizeName])
	assert.Equal(t, attachment.AttachmentProperties[DeviceName], unmarshalledAttachment.AttachmentProperties[DeviceName])
	assert.Equal(t, attachment.AttachmentProperties[VolumeIdName], unmarshalledAttachment.AttachmentProperties[VolumeIdName])
	assert.Equal(t, attachment.AttachmentProperties[FileSystemTypeName], unmarshalledAttachment.AttachmentProperties[FileSystemTypeName])

	expectedExpiresAtUTC, err := time.Parse(time.RFC3339, attachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	unmarshalledExpiresAtUTC, err := time.Parse(time.RFC3339, unmarshalledAttachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Equal(t, expectedExpiresAtUTC, unmarshalledExpiresAtUTC)
}

func TestStartTimerErrorWhenExpiresAtIsInThePast(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           status.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
	}
	assert.Error(t, attachment.StartTimer(func() {}))
}

func TestHasExpired(t *testing.T) {
	for _, tc := range []struct {
		expiresAt int64
		expected  bool
		name      string
	}{
		{time.Now().Unix() - 1, true, "expiresAt in past returns true"},
		{time.Now().Unix() + 10, false, "expiresAt in future returns false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attachment := &ResourceAttachment{
				AttachmentInfo: attachmentinfo.AttachmentInfo{
					TaskARN:          taskARN,
					AttachmentARN:    attachmentARN,
					AttachStatusSent: attachSent,
					Status:           status.AttachmentNone,
					ExpiresAt:        time.Unix(tc.expiresAt, 0),
				},
				AttachmentProperties: testAttachmentProperties,
			}
			assert.Equal(t, tc.expected, attachment.HasExpired())
		})
	}
}

func TestInitialize(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	timeoutFunc := func() {
		wg.Done()
	}

	expiresAt := time.Now().Unix() + 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        status.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
	}
	assert.NoError(t, attachment.Initialize(timeoutFunc))
	wg.Wait()
}

func TestInitializeExpired(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        status.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
	}
	assert.Error(t, attachment.Initialize(func() {}))
}

func TestInitializeExpiredButAlreadySent(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           status.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
	}
	assert.NoError(t, attachment.Initialize(func() {}))
}
