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

package networkinterface

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN        = "t1"
	attachmentARN  = "att1"
	mac            = "mac1"
	attachSent     = true
	attachmentType = "eni"
)

func TestMarshalUnmarshal(t *testing.T) {
	expiresAt := time.Now()
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        expiresAt,
		},
		MACAddress: mac,
	}
	bytes, err := json.Marshal(attachment)
	assert.NoError(t, err)
	var unmarshalledAttachment ENIAttachment
	err = json.Unmarshal(bytes, &unmarshalledAttachment)
	assert.NoError(t, err)
	assert.Equal(t, attachment.TaskARN, unmarshalledAttachment.TaskARN)
	assert.Equal(t, attachment.AttachmentARN, unmarshalledAttachment.AttachmentARN)
	assert.Equal(t, attachment.AttachStatusSent, unmarshalledAttachment.AttachStatusSent)
	assert.Equal(t, attachment.MACAddress, unmarshalledAttachment.MACAddress)
	assert.Equal(t, attachment.Status, unmarshalledAttachment.Status)

	expectedExpiresAtUTC, err := time.Parse(time.RFC3339, attachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	unmarshalledExpiresAtUTC, err := time.Parse(time.RFC3339, unmarshalledAttachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Equal(t, expectedExpiresAtUTC, unmarshalledExpiresAtUTC)
}

func TestMarshalUnmarshalWithAttachmentType(t *testing.T) {
	expiresAt := time.Now()
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        expiresAt,
		},
		AttachmentType: attachmentType,
		MACAddress:     mac,
	}
	bytes, err := json.Marshal(attachment)
	assert.NoError(t, err)
	var unmarshalledAttachment ENIAttachment
	err = json.Unmarshal(bytes, &unmarshalledAttachment)
	assert.NoError(t, err)
	assert.Equal(t, attachment.AttachmentType, unmarshalledAttachment.AttachmentType)
	assert.Equal(t, attachment.TaskARN, unmarshalledAttachment.TaskARN)
	assert.Equal(t, attachment.AttachmentARN, unmarshalledAttachment.AttachmentARN)
	assert.Equal(t, attachment.AttachStatusSent, unmarshalledAttachment.AttachStatusSent)
	assert.Equal(t, attachment.MACAddress, unmarshalledAttachment.MACAddress)
	assert.Equal(t, attachment.Status, unmarshalledAttachment.Status)

	expectedExpiresAtUTC, err := time.Parse(time.RFC3339, attachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	unmarshalledExpiresAtUTC, err := time.Parse(time.RFC3339, unmarshalledAttachment.ExpiresAt.Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Equal(t, expectedExpiresAtUTC, unmarshalledExpiresAtUTC)
}

func TestStartTimerErrorWhenExpiresAtIsInThePast(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		MACAddress: mac,
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
			attachment := &ENIAttachment{
				AttachmentInfo: attachment.AttachmentInfo{
					TaskARN:          taskARN,
					AttachmentARN:    attachmentARN,
					AttachStatusSent: attachSent,
					Status:           attachment.AttachmentNone,
					ExpiresAt:        time.Unix(tc.expiresAt, 0),
				},
				MACAddress: mac,
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
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        attachment.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		MACAddress: mac,
	}
	assert.NoError(t, attachment.Initialize(timeoutFunc))
	wg.Wait()
}

func TestInitializeExpired(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:       taskARN,
			AttachmentARN: attachmentARN,
			Status:        attachment.AttachmentNone,
			ExpiresAt:     time.Unix(expiresAt, 0),
		},
		MACAddress: mac,
	}
	assert.Error(t, attachment.Initialize(func() {}))
}

func TestInitializeExpiredButAlreadySent(t *testing.T) {
	expiresAt := time.Now().Unix() - 1
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        time.Unix(expiresAt, 0),
		},
		MACAddress: mac,
	}
	assert.NoError(t, attachment.Initialize(func() {}))
}

// TestMarshalUnmarshalWithInterfaceConfig verifies the interface
// configuration carried on the attachment survives a persistence round trip
// and still builds a usable interface model, so an attachment restored from
// the data store can drive interface configuration after a restart.
func TestMarshalUnmarshalWithInterfaceConfig(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute)
	attachment := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
			ExpiresAt:        expiresAt,
		},
		AttachmentType: ENIAttachmentTypeTaskENI,
		MACAddress:     mac,
		InterfaceConfig: &ecsacs.ElasticNetworkInterface{
			Ec2Id:                        aws.String("eni-12345"),
			MacAddress:                   aws.String(mac),
			Name:                         aws.String("eth1"),
			Index:                        aws.Int64(0),
			Ipv4Addresses:                []*ecsacs.IPv4AddressAssignment{{Primary: aws.Bool(true), PrivateAddress: aws.String("10.0.0.1")}},
			Ipv6Addresses:                []*ecsacs.IPv6AddressAssignment{{Address: aws.String("2001:db8::1")}},
			SubnetGatewayIpv4Address:     aws.String("10.0.0.0/24"),
			SubnetGatewayIpv6Address:     aws.String("2001:db8::/64"),
			DomainNameServers:            []*string{aws.String("10.0.0.2")},
			DomainName:                   []*string{aws.String("us-west-2.compute.internal")},
			PrivateDnsName:               aws.String("ip-10-0-0-1.us-west-2.compute.internal"),
			InterfaceAssociationProtocol: aws.String(DefaultInterfaceAssociationProtocol),
		},
	}

	marshalled, err := json.Marshal(attachment)
	assert.NoError(t, err)

	var unmarshalled ENIAttachment
	assert.NoError(t, json.Unmarshal(marshalled, &unmarshalled))
	require.NotNil(t, unmarshalled.InterfaceConfig)

	// The restored configuration must still build the same interface model a
	// task payload would, including the fields derived from host state.
	macToName := map[string]string{mac: "eth1"}
	original, err := New(attachment.InterfaceConfig, "", nil, macToName)
	require.NoError(t, err)
	restored, err := New(unmarshalled.InterfaceConfig, "", nil, macToName)
	require.NoError(t, err)
	assert.Equal(t, original, restored)

	assert.Equal(t, "eni-12345", restored.ID)
	assert.Equal(t, "eth1", restored.DeviceName, "device name must come from host state")
	assert.Equal(t, "eth1", restored.Name)
	assert.Equal(t, "10.0.0.0/24", restored.SubnetGatewayIPV4Address)
	assert.Equal(t, []string{"10.0.0.2"}, restored.DomainNameServers)
}

// TestUnmarshalOldRecordWithoutInterface verifies that an attachment record
// persisted before the interface configuration field existed still restores
// cleanly, with no interface configuration. An absent configuration marshals
// to the exact pre-change wire format (the field is omitted), which is also
// asserted so that new-agent records remain readable by older readers.
func TestUnmarshalOldRecordWithoutInterface(t *testing.T) {
	oldFormat := &ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: attachSent,
			Status:           attachment.AttachmentNone,
		},
		AttachmentType: ENIAttachmentTypeTaskENI,
		MACAddress:     mac,
	}

	oldRecord, err := json.Marshal(oldFormat)
	assert.NoError(t, err)
	// The absent configuration must not appear on the wire (pre-change format).
	assert.NotContains(t, string(oldRecord), "interfaceConfig")

	var unmarshalled ENIAttachment
	assert.NoError(t, json.Unmarshal(oldRecord, &unmarshalled))
	assert.Equal(t, taskARN, unmarshalled.TaskARN)
	assert.Equal(t, mac, unmarshalled.MACAddress)
	assert.Nil(t, unmarshalled.InterfaceConfig)
}
