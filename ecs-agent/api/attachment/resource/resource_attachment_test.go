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

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"

	"github.com/stretchr/testify/assert"
)

const (
	taskARN       = "t1"
	attachmentARN = "att1"
	attachSent    = true
)

var (
	testAttachmentProperties = map[string]string{
		VolumeNameKey:           "myCoolVolume",
		SourceVolumeHostPathKey: "/testPath",
		VolumeSizeGibKey:        "7",
		DeviceNameKey:           "/dev/nvme0n0",
		VolumeIdKey:             "vol-123",
		FileSystemKey:           "testXFS",
	}
)

func TestMarshalUnmarshal(t *testing.T) {
	expiresAt := time.Now()
	attachment := &ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        expiresAt,
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       EBSTaskAttach,
	}
	bytes, err := json.Marshal(attachment)
	assert.NoError(t, err)
	var unmarshalledAttachment ResourceAttachment
	err = json.Unmarshal(bytes, &unmarshalledAttachment)
	assert.NoError(t, err)
	assert.Equal(t, attachment.TaskARN, unmarshalledAttachment.TaskARN)
	assert.Equal(t, attachment.AttachmentARN, unmarshalledAttachment.AttachmentARN)
	assert.Equal(t, attachment.AttachStatusSent, unmarshalledAttachment.AttachStatusSent)
	assert.Equal(t, attachment.AttachmentType, unmarshalledAttachment.AttachmentType)
	assert.Equal(t, attachment.Status, unmarshalledAttachment.Status)

	assert.Equal(t, attachment.AttachmentProperties[VolumeNameKey], unmarshalledAttachment.AttachmentProperties[VolumeNameKey])
	assert.Equal(t, attachment.AttachmentProperties[SourceVolumeHostPathKey], unmarshalledAttachment.AttachmentProperties[SourceVolumeHostPathKey])
	assert.Equal(t, attachment.AttachmentProperties[VolumeSizeGibKey], unmarshalledAttachment.AttachmentProperties[VolumeSizeGibKey])
	assert.Equal(t, attachment.AttachmentProperties[DeviceNameKey], unmarshalledAttachment.AttachmentProperties[DeviceNameKey])
	assert.Equal(t, attachment.AttachmentProperties[VolumeIdKey], unmarshalledAttachment.AttachmentProperties[VolumeIdKey])
	assert.Equal(t, attachment.AttachmentProperties[FileSystemKey], unmarshalledAttachment.AttachmentProperties[FileSystemKey])

	expectedExpiresAtUTC, err := time.Parse(time.RFC3339, attachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	unmarshalledExpiresAtUTC, err := time.Parse(time.RFC3339, unmarshalledAttachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Equal(t, expectedExpiresAtUTC, unmarshalledExpiresAtUTC)
}

func TestStartTimerErrorWhenExpiresAtIsInThePast(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       EBSTaskAttach,
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
				AttachmentInfo: attachment.AttachmentInfo{
					TaskARN:          taskARN,
					AttachmentARN:    attachmentARN,
					AttachStatusSent: attachSent,
					Status:           attachment.AttachmentNone,
					ExpiresAt:        time.Unix(tc.expiresAt, 0),
				},
				AttachmentProperties: testAttachmentProperties,
				AttachmentType:       EBSTaskAttach,
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
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        attachment.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       EBSTaskAttach,
	}
	assert.NoError(t, attachment.Initialize(timeoutFunc))
	wg.Wait()
}

func TestInitializeExpired(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        attachment.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       EBSTaskAttach,
	}
	assert.Error(t, attachment.Initialize(func() {}))
}

func TestInitializeExpiredButAlreadySent(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ResourceAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		AttachmentProperties: testAttachmentProperties,
		AttachmentType:       EBSTaskAttach,
	}
	assert.NoError(t, attachment.Initialize(func() {}))
}

// TestGetVolumeSpecificPropertiesForEBSAttach ensures all required properties for EBS attach are correct.
func TestGetVolumeSpecificPropertiesForEBSAttach(t *testing.T) {
	expected := []string{
		"volumeId",
		"volumeSizeGib",
		"deviceName",
		"sourceVolumeHostPath",
		"volumeName",
	}
	actual := GetVolumeSpecificPropertiesForEBSAttach()

	assert.Equal(t, expected, actual)
}
