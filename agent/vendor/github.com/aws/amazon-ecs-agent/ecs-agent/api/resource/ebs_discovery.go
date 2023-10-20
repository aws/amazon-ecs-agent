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
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ebsVolumeDiscoveryTimeout = 300 * time.Second
	ebsResourceKeyPrefix      = "ebs-volume:"
	ScanPeriod                = 500 * time.Millisecond
	deviceNamePrefix          = "/dev"
)

var (
	// When confirming an EBS volume is attached to a host, if the expected volume ID does not
	// match the volume ID found on the host, this error is returned.
	ErrInvalidVolumeID = errors.New("EBS volume IDs do not match")
)

type EBSDiscoveryClient struct {
	ctx           context.Context
	hasXenSupport bool
}

type EBSDiscoveryClientOption func(*EBSDiscoveryClient)

// Enable Xen instances support for EBS Discovery Client
func WithXenSupport() EBSDiscoveryClientOption {
	return func(ec *EBSDiscoveryClient) {
		ec.hasXenSupport = true
	}
}

func NewDiscoveryClient(ctx context.Context, opts ...EBSDiscoveryClientOption) *EBSDiscoveryClient {
	client := &EBSDiscoveryClient{
		ctx: ctx,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func (client *EBSDiscoveryClient) HasXenSupport() bool {
	return client.hasXenSupport
}

// ScanEBSVolumes will iterate through the entire list of provided EBS volume attachments within the agent state and checks if it's attached on the host.
func ScanEBSVolumes[T GenericEBSAttachmentObject](pendingAttachments map[string]T, dc EBSDiscovery) map[string]string {
	foundVolumes := make(map[string]string)
	for key, ebs := range pendingAttachments {
		volumeId := strings.TrimPrefix(key, ebsResourceKeyPrefix)
		deviceName := ebs.GetAttachmentProperties(DeviceNameKey)
		actualDeviceName, err := dc.ConfirmEBSVolumeIsAttached(deviceName, volumeId)
		if err != nil {
			if !errors.Is(err, ErrInvalidVolumeID) {
				err = fmt.Errorf("%w; failed to confirm if EBS volume is attached to the host", err)
			}
			ebs.SetError(err)
			continue
		}
		foundVolumes[volumeId] = actualDeviceName
	}
	return foundVolumes
}
