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
	PROTOCOL        = "unix"
	fsTypeBlockName = "block"
)

// csiClient encapsulates all CSI methods.
type csiClient struct {
	csiSocket string
}

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
	// TODO extract server setup into separate function
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
		return err
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)
	defaultVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
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

	//_, err = client.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
	//	VolumeId: "vol-03bde5a27631ad16b",
	//	PublishContext: map[string]string{
	//		"devicePath": "/dev/nvme1n1",
	//	},
	//	StagingTargetPath: "/testebs",
	//
	//		VolumeCapability: &csi.VolumeCapability{
	//			AccessType: &csi.VolumeCapability_Mount{},
	//			AccessMode: &csi.VolumeCapability_AccessMode{
	//				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	//			},
	//		},
	//	})

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
