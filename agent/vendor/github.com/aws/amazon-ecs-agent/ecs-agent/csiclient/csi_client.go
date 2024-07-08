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
	"path/filepath"
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

func DefaultSocketFilePath() string {
	return filepath.Join(DefaultSocketHostPath, DefaultImageName, DefaultSocketName)
}

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
	NodeUnstageVolume(ctx context.Context, volumeId, stagingTargetPath string) error
	GetVolumeMetrics(ctx context.Context, volumeId string, hostMountPath string) (*Metrics, error)
	NodeGetCapabilities(ctx context.Context) (*csi.NodeGetCapabilitiesResponse, error)
}

// csiClient encapsulates all CSI methods.
type csiClient struct {
	csiSocket string
}

// NewCSIClient creates a CSI client for the communication with CSI driver daemon.
func NewCSIClient(socketIn string) csiClient {
	return csiClient{csiSocket: socketIn}
}

// Returns a CSI client configured with default settings.
// The default socket filepath is defined in the respective DefaultSocketFilePath method
// for each platform (linux/windows).
func NewDefaultCSIClient() CSIClient {
	client := NewCSIClient(DefaultSocketFilePath())
	return &client
}

// NodeStageVolume will do following things for the given volume:
// 1. format the device if it does not have any,
// 2. mount the device to given stagingTargetPath,
// 3. resize the fs if its size is smaller than the device size.
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
	conn, err := cc.grpcDialConnect(ctx)
	if err != nil {
		return fmt.Errorf("NodeStageVolume: failed to establish CSI connection: %w", err)
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
		return fmt.Errorf("failed to stage volume via CSI driver: %w", err)
	}
	return nil
}

// NodeUnstageVolume will unstage/umount the given volume from the stagingTargetPath.
func (cc *csiClient) NodeUnstageVolume(ctx context.Context, volumeId, stagingTargetPath string) error {
	conn, err := cc.grpcDialConnect(ctx)
	if err != nil {
		return fmt.Errorf("NodeUnstageVolume: failed to establish CSI connection: %w", err)
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)
	_, err = client.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId:          volumeId,
		StagingTargetPath: stagingTargetPath,
	})
	if err != nil {
		return fmt.Errorf("failed to unstage volume via CSI driver: %w", err)
	}
	return nil
}

// GetVolumeMetrics returns volume usage.
func (cc *csiClient) GetVolumeMetrics(ctx context.Context, volumeId string, hostMountPath string) (*Metrics, error) {
	conn, err := cc.grpcDialConnect(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetVolumeMetrics: failed to establish CSI connection: %w", err)
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)
	resp, err := client.NodeGetVolumeStats(ctx, &csi.NodeGetVolumeStatsRequest{
		VolumeId:   volumeId,
		VolumePath: hostMountPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get stats via CSI driver: %w", err)
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

// Gets node capabilities of the EBS CSI Driver
func (cc *csiClient) NodeGetCapabilities(ctx context.Context) (*csi.NodeGetCapabilitiesResponse, error) {
	conn, err := cc.grpcDialConnect(ctx)
	if err != nil {
		logger.Error("NodeGetCapabilities: CSI Connection Error", logger.Fields{field.Error: err})
		return nil, err
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)
	resp, err := client.NodeGetCapabilities(ctx, &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		logger.Error("Could not get EBS CSI node capabilities", logger.Fields{field.Error: err})
		return nil, err
	}

	return resp, nil
}

func (cc *csiClient) grpcDialConnect(ctx context.Context) (*grpc.ClientConn, error) {
	dialer := func(addr string, t time.Duration) (net.Conn, error) {
		return net.Dial(protocol, addr)
	}
	conn, err := grpc.DialContext(
		ctx,
		cc.csiSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDialer(dialer),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"error building a connection to CSI driver with socket %s: %w", cc.csiSocket, err)
	}
	return conn, nil
}
