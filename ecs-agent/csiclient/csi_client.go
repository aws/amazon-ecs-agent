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

package csiclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PROTOCOL = "unix"
)

// CSIClient is an interface that specifies all supported operations in the Container Storage Interface(CSI)
// driver for Agent uses. The CSI driver provides many volume related operations to manage the lifecycle of
// Amazon EBS volumes, including mounting, umounting, resizing and volume stats.
type CSIClient interface {
	GetVolumeMetrics(volumeId string, hostMountPath string) (*Metrics, error)
}

// csiClient encapsulates all CSI methods.
type csiClient struct {
	csiSocket string
}

// NewCSIClient creates a CSI client for the communication with CSI driver daemon.
func NewCSIClient(socketIn string) CSIClient {
	return &csiClient{csiSocket: socketIn}
}

// GetVolumeMetrics returns volume usage.
func (cc *csiClient) GetVolumeMetrics(volumeId string, hostMountPath string) (*Metrics, error) {
	// Set up a connection to the server
	dialer := func(addr string, t time.Duration) (net.Conn, error) {
		return net.Dial(PROTOCOL, addr)
	}
	conn, err := grpc.Dial(
		cc.csiSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDialer(dialer),
	)
	if err != nil {
		logger.Error("Error building a connection to CSI driver", logger.Fields{
			field.Error: err,
			"Socket":    cc.csiSocket,
		})
		return nil, err
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, err := client.NodeGetVolumeStats(ctx, &csi.NodeGetVolumeStatsRequest{
		VolumeId:   volumeId,
		VolumePath: hostMountPath,
	})
	if err != nil {
		logger.Error("Could not get stats", logger.Fields{
			field.Error: err,
		})
		return nil, err
	}

	usages := resp.GetUsage()
	if usages == nil {
		return nil, fmt.Errorf("failed to get usage from response because the usage is nil")
	}

	var usedBytes, totalBytes int64
	for _, usage := range usages {
		unit := usage.GetUnit()
		switch unit {
		case csi.VolumeUsage_BYTES:
			usedBytes = usage.GetUsed()
			totalBytes = usage.GetTotal()
			logger.Debug("Found volume usage", logger.Fields{
				"UsedBytes":  usedBytes,
				"TotalBytes": totalBytes,
			})
		case csi.VolumeUsage_INODES:
			logger.Debug("Ignore inodes key")
		default:
			logger.Warn("Found unknown key in volume usage", logger.Fields{
				"Unit": unit,
			})
		}
	}
	return &Metrics{
		Used:     usedBytes,
		Capacity: totalBytes,
	}, nil
}
