// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
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
	v1 "k8s.io/api/core/v1"
)

const (
	protocol        = "unix"
	fsTypeBlockName = "block"
)

// CSIClient is an interface that specifies all supported operations in the Container Storage Interface(CSI)
// driver for Agent uses. The CSI driver provides many volume related operations to manage the lifecycle of
// Amazon EBS volumes, including mounting, umounting, resizing and volume stats.
type CSIClient interface {
	NodeStageVolume(ctx context.Context,
		volID string,
		publishContext map[string]string,
		stagingTargetPath string,
		fsType string,
		accessMode v1.PersistentVolumeAccessMode,
		secrets map[string]string,
		volumeContext map[string]string,
		mountOptions []string,
		fsGroup *int64,
	) error
	GetVolumeMetrics(volumeId string, hostMountPath string) (*Metrics, error)
}

// csiClient encapsulates all CSI methods.
type csiClient struct {
	csiSocket string
}

// NewCSIClient creates a CSI client for the communication with CSI driver daemon.
func NewCSIClient(socketIn string) csiClient {
	return csiClient{csiSocket: socketIn}
}

func (cc *csiClient) NodeStageVolume(ctx context.Context,
	volID string,
	publishContext map[string]string,
	stagingTargetPath string,
	fsType string,
	accessMode v1.PersistentVolumeAccessMode,
	secrets map[string]string,
	volumeContext map[string]string,
	mountOptions []string,
	fsGroup *int64,
) error {
	conn, err := cc.grpcDialConnect()
	if err != nil {
		logger.Error("NodeStage: CSI Connection Error", logger.Fields{
			field.Error: err,
		})
		return err
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)

	defaultVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	req := csi.NodeStageVolumeRequest{
		VolumeId:          volID,
		PublishContext:    publishContext,
		StagingTargetPath: stagingTargetPath,
		VolumeCapability:  defaultVolumeCapability,
		Secrets:           secrets,
		VolumeContext:     volumeContext,
	}

	if fsType == fsTypeBlockName {
		req.VolumeCapability.AccessType = &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		}
	} else {
		mountVolume := &csi.VolumeCapability_MountVolume{
			FsType:     fsType,
			MountFlags: mountOptions,
		}
		req.VolumeCapability.AccessType = &csi.VolumeCapability_Mount{
			Mount: mountVolume,
		}
	}

	_, err = client.NodeStageVolume(ctx, &req)

	if err != nil {
		logger.Error("Error staging volume via CSI driver", logger.Fields{
			field.Error: err,
		})
		return err
	}
	return nil
}

// GetVolumeMetrics returns volume usage.
func (cc *csiClient) GetVolumeMetrics(volumeId string, hostMountPath string) (*Metrics, error) {
	conn, err := cc.grpcDialConnect()
	if err != nil {
		logger.Error("GetVolumeMetrics: CSI Connection Error")
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

func (cc *csiClient) grpcDialConnect() (*grpc.ClientConn, error) {
	dialer := func(addr string, t time.Duration) (net.Conn, error) {
		return net.Dial(protocol, addr)
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
	return conn, nil
}
