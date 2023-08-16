//go:build windows
// +build windows

package resource

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

const (
	ebsnvmeIDTimeoutDuration = 5 * time.Second
)

var (
	ErrInvalidVolumeID = errors.New("EBS volume IDs do not match")
)

type EBSDiscoveryClient struct {
	ctx context.Context
}

func NewDiscoveryClient(ctx context.Context) EBSDiscovery {
	return &EBSDiscoveryClient{
		ctx: ctx,
	}
}

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	return nil
}
