package resource

import (
	"context"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/pkg/errors"
)

const (
	ebsnvmeIDTimeoutDuration = 5 * time.Second
	ebsResourceKeyPrefix     = "ebs-volume:"
	ScanPeriod               = 500 * time.Millisecond
)

var (
	ErrInvalidVolumeID = errors.New("EBS volume IDs do not match")
)

type EBSDiscoveryClient struct {
	ctx context.Context
}

type ScanTickerController struct {
	ScanTicker *time.Ticker
	Running    bool
	TickerLock sync.Mutex
	Done       chan bool
}

func NewDiscoveryClient(ctx context.Context) *EBSDiscoveryClient {
	return &EBSDiscoveryClient{
		ctx: ctx,
	}
}

func NewScanTickerController() *ScanTickerController {
	return &ScanTickerController{
		ScanTicker: nil,
		Running:    false,
		TickerLock: sync.Mutex{},
		Done:       make(chan bool),
	}
}

func (c *ScanTickerController) StopScanTicker() {
	c.TickerLock.Lock()
	defer c.TickerLock.Unlock()
	if !c.Running {
		return
	}
	log.Info("No more attachments to scan for. Stopping scan ticker.")
	c.Done <- true
}

func ScanEBSVolumes[T GenericEBSAttachmentObject](pendingAttachments map[string]T, dc EBSDiscovery) []string {
	var err error
	var foundVolumes []string
	for key, ebs := range pendingAttachments {
		volumeId := strings.TrimPrefix(key, ebsResourceKeyPrefix)
		deviceName := ebs.GetAttachmentProperties(DeviceName)
		err = dc.ConfirmEBSVolumeIsAttached(deviceName, volumeId)
		if err != nil {
			if err == ErrInvalidVolumeID || errors.Cause(err) == ErrInvalidVolumeID {
				log.Warnf("Expected EBS volume with device name: %v and volume ID: %v, Found a different EBS volume attached to the host.", deviceName, volumeId)
			} else {
				log.Warnf("Failed to confirm if EBS volume with volume ID: %v and device name: %v, is attached to the host. Error: %v", volumeId, deviceName, err)
			}
			continue
		}
		foundVolumes = append(foundVolumes, key)
	}
	return foundVolumes
}
