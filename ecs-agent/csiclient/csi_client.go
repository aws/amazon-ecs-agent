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

type csiClient struct {
	csiSocket string
}

func NewCSIClient(socketIn string) csiClient {
	return csiClient{csiSocket: socketIn}
}

// Used for testing and integration
// TODO update with stats packing
func (cc *csiClient) GetVolumeMetrics(volumeId string, hostMountPath string) (int64, error) {
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
		logger.Info("Error publishing metrics", logger.Fields{
			field.Error: err,
		})
		return int64(0), err
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
		logger.Info("could not get stats", logger.Fields{
			field.Error: err,
		})
		return int64(0), err
	}
	usages := resp.GetUsage()
	// TODO update return type and values to match TCS payload
	if usages == nil {
		return int64(0), fmt.Errorf("failed to get usage from response. usage is nil")
	}
	var result = int64(0)
	for _, usage := range usages {
		unit := usage.GetUnit()
		switch unit {
		case csi.VolumeUsage_BYTES:
			result = usage.GetUsed()
		default:
			logger.Info("Found missing key in volume usage")
		}
	}
	return result, nil
}
