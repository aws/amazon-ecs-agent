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

package ebs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	agentState dockerstate.TaskEngineState
	// TODO: The dataClient will be used to save to agent's data client as well as start the ACK timer. This will be added once the data client functionality have been added
	// dataClient     data.Client
	ebsChangeEvent       chan<- statechange.Event
	discoveryClient      apiebs.EBSDiscovery
	scanTickerController *ScanTickerController
}

type ScanTickerController struct {
	scanTicker *time.Ticker
	running    bool
	tickerLock sync.Mutex
	done       chan bool
}

// NewWatcher is used to return a new instance of the EBSWatcher struct
func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*EBSWatcher, error) {
	derivedContext, cancel := context.WithCancel(ctx)
	discoveryClient := apiebs.NewDiscoveryClient(derivedContext)
	scanTickerController := NewScanTickerController()
	return &EBSWatcher{
		ctx:                  derivedContext,
		cancel:               cancel,
		agentState:           state,
		ebsChangeEvent:       stateChangeEvents,
		discoveryClient:      discoveryClient,
		scanTickerController: scanTickerController,
	}, nil
}

func NewScanTickerController() *ScanTickerController {
	return &ScanTickerController{
		scanTicker: nil,
		running:    false,
		tickerLock: sync.Mutex{},
	}
}

func (w *EBSWatcher) Start() {
	w.scanTickerController.tickerLock.Lock()
	defer w.scanTickerController.tickerLock.Unlock()

	if w.scanTickerController.running || len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		return
	}

	w.scanTickerController.running = true
	w.scanTickerController.scanTicker = time.NewTicker(apiebs.ScanPeriod)

	log.Info("New resource attachment to handle. Starting EBS watcher.")
	go func() {
		for {
			select {
			case <-w.scanTickerController.scanTicker.C:
				pendingEBS := w.agentState.GetAllPendingEBSAttachmentWithKey()
				foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, w.discoveryClient)
				w.NotifyFound(foundVolumes)
			case <-w.scanTickerController.done:
				w.scanTickerController.running = false
				w.scanTickerController.scanTicker.Stop()
			case <-w.ctx.Done():
				w.scanTickerController.StopScanTicker()
				log.Info("EBS Watcher Stopped due to agent stop")
				return
			}
		}
	}()
	log.Info("EBS watcher started.")
}

// Stop will stop the EBS watcher
func (w *EBSWatcher) Stop() {
	log.Info("Stopping EBS watcher.")
	w.cancel()
}

func (c *ScanTickerController) StopScanTicker() {
	c.tickerLock.Lock()
	defer c.tickerLock.Unlock()
	if !c.running {
		return
	}
	c.done <- true
}

func (w *EBSWatcher) HandleResourceAttachment(ebs *apiebs.ResourceAttachment) error {
	empty := len(w.agentState.GetAllPendingEBSAttachments()) == 0
	attachmentType := ebs.AttachmentProperties[apiebs.ResourceTypeName]
	if attachmentType != apiebs.ElasticBlockStorage {
		log.Info("Resource type not Elastic Block Storage. Skip handling resource attachment.")
		return nil
	}
	volumeID := ebs.AttachmentProperties[apiebs.VolumeIdName]
	_, ok := w.agentState.GetEBSByVolumeId(volumeID)
	if ok {
		log.Infof("EBS Volume attachment already exists. Skip handling EBS attachment %v.", ebs.String())
		return nil
	}

	if err := w.addEBSAttachmentToState(ebs); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("attach %s message handler: unable to add ebs attachment to engine state: %s",
			attachmentType, ebs.String()))
	}

	if empty && len(w.agentState.GetAllPendingEBSAttachments()) == 1 {
		go w.Start()
		// time.Sleep(5 * time.Millisecond)
	}

	return nil
}

func (w *EBSWatcher) NotifyFound(foundVolumes []string) {
	for _, volumeId := range foundVolumes {
		w.notifyFoundEBS(volumeId)
	}
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		w.scanTickerController.StopScanTicker()
	}
}

func (w *EBSWatcher) notifyFoundEBS(volumeId string) {
	ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Unable to find EBS volume with volume ID: %v.", volumeId)
		return
	}

	if ebs.HasExpired() {
		log.Warnf("EBS status expired, no longer tracking EBS volume: %v.", ebs.String())
		return
	}

	// TODO: This is a placeholder for now until the attachment ACS handler gets implemented
	if ebs.IsSent() {
		log.Warnf("State change event has already been emitted for EBS volume: %v.", ebs.String())
		return
	}

	if ebs.IsAttached() {
		log.Infof("EBS volume: %v, has been found already.", ebs.String())
		return
	}

	ebs.StopAckTimer()
	ebs.SetAttachedStatus()

	log.Infof("Successfully found attached EBS volume: %v", ebs.String())
}

func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	// TODO: Remove the EBS volume from the data client.
	w.agentState.RemoveEBSAttachment(volumeID)
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		log.Info("No more attachments to scan for. Stopping scan ticker.")
		w.scanTickerController.StopScanTicker()
	}
}

// addEBSAttachmentToState adds an EBS attachment to state, and start its ack timer
func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
	volumeId := string(ebs.AttachmentProperties[apiebs.VolumeIdName])
	err := ebs.StartTimer(func() {
		w.handleEBSAckTimeout(volumeId)
	})
	if err != nil {
		return err
	}

	w.agentState.AddEBSAttachment(ebs)
	return nil
}

// handleEBSAckTimeout removes EBS attachment from agent state after the EBS ack timeout
func (w *EBSWatcher) handleEBSAckTimeout(volumeId string) {
	ebsAttachment, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Ignoring unmanaged EBS attachment volume ID=%s", volumeId)
		return
	}
	if !ebsAttachment.IsSent() {
		log.Warnf("Timed out waiting for EBS ack; removing EBS attachment record %v", ebsAttachment.String())
		// w.RemoveAttachment(volumeId)
		w.removeEBSAttachment(volumeId)
	}
}
