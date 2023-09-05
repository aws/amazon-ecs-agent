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
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/pkg/errors"
)

const (
	ebsVolumeDiscoveryTimeout = 5 * time.Second
	ebsResourceKeyPrefix      = "ebs-volume:"
	ScanPeriod                = 500 * time.Millisecond
)

var (
	ErrInvalidVolumeID = errors.New("EBS volume IDs do not match")
)

type EBSDiscoveryClient struct {
	ctx context.Context
}

func NewDiscoveryClient(ctx context.Context) *EBSDiscoveryClient {
	return &EBSDiscoveryClient{
		ctx: ctx,
	}
}

// ScanEBSVolumes will iterate through the entire list of pending EBS volume attachments within the agent state and checks if it's attached on the host.
func ScanEBSVolumes[T GenericEBSAttachmentObject](t map[string]T, dc EBSDiscovery) []string {
	var err error
	var foundVolumes []string
	for key, ebs := range t {
		volumeId := strings.TrimPrefix(key, ebsResourceKeyPrefix)
		deviceName := ebs.GetAttachmentProperties(DeviceName)
		err = dc.ConfirmEBSVolumeIsAttached(deviceName, volumeId)
		if err != nil {
			if err == ErrInvalidVolumeID || errors.Cause(err) == ErrInvalidVolumeID {
				logger.Warn("Found a different EBS volume attached to the host. Expected EBS volume:", logger.Fields{
					"volumeId":   volumeId,
					"deviceName": deviceName,
				})
			} else {
				logger.Warn("Failed to confirm if EBS volume is attached to the host. ", logger.Fields{
					"volumeId":   volumeId,
					"deviceName": deviceName,
					"error":      err,
				})
			}
			continue
		}
		foundVolumes = append(foundVolumes, key)
	}
	return foundVolumes
}
