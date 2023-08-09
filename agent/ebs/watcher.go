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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	log "github.com/cihub/seelog"

	"github.com/pkg/errors"
)

const (
	// // sendEBSStateChangeRetryTimeout specifies the timeout before giving up
	// // when looking for EBS in agent's state. If for whatever reason, the message
	// // from ACS is received after the EBS has been attached to the instance, this
	// // timeout duration will be used to wait for EBS message to be sent from ACS
	// sendEBSStateChangeRetryTimeout = 6 * time.Second

	// // sendEBSStateChangeBackoffMin specifies minimum value for backoff when
	// // waiting for attachment message from ACS
	// sendEBSStateChangeBackoffMin = 100 * time.Millisecond

	// // sendEBSStateChangeBackoffMax specifies maximum value for backoff when
	// // waiting for attachment message from ACS
	// sendEBSStateChangeBackoffMax = 250 * time.Millisecond

	// // sendEBSStateChangeBackoffJitter specifies the jitter multiple percentage
	// // when waiting for attachment message from ACS
	// sendEBSStateChangeBackoffJitter = 0.2

	// // sendEBSStateChangeBackoffMultiple specifies the backoff duration multipler
	// // when waiting for the attachment message from ACS
	// sendEBSStateChangeBackoffMultiple = 1.5

	// // volumeIDRetryTimeout specifies the timeout before giving up when
	// // looking for an EBS's volume ID on the host.
	// // We are capping off this duration to 1s assuming worst-case behavior
	// volumeIDRetryTimeout = 2 * time.Second

	// // ebsStatusSentMsg is the error message to use when trying to send an ebs status that's
	// // already been sent
	// ebsStatusSentMsg = "ebs status already sent"

	scanPeriod = 500 * time.Millisecond
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	scanTicker *time.Ticker
	agentState dockerstate.TaskEngineState
	// dataClient will be used to save to agent's data client as well as start the ACK timer, perhaps this will be apart of another stuct called EBSHandler
	// dataClient     data.Client
	ebsChangeEvent chan<- statechange.Event
	mailbox        chan func()
}

func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*EBSWatcher, error) {
	derivedContext, cancel := context.WithCancel(ctx)
	log.Info("ebs watcher has been initialized")
	return &EBSWatcher{
		ctx:            derivedContext,
		cancel:         cancel,
		agentState:     state,
		ebsChangeEvent: stateChangeEvents,
		mailbox:        make(chan func(), 1),
	}, nil
}

func (w *EBSWatcher) Start() {
	log.Info("Starting EBS watcher")

	w.scanTicker = time.NewTicker(scanPeriod)

	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		log.Info("EBS watcher stopping")
		w.scanTicker.Stop()
	}

	for {
		select {
		case f := <-w.mailbox:
			log.Info("EBS watcher received function")
			f()
		case <-w.scanTicker.C:
			log.Info("EBS watcher about to scan")
			w.scanEBSVolumes()
		case <-w.ctx.Done():
			w.scanTicker.Stop()
			log.Info("EBS watcher stopped")
			return
		}
	}
}

func (w *EBSWatcher) Stop() {
	log.Info("Stopping EBS watcher")
	w.cancel()
}

func (w *EBSWatcher) HandleResourceAttachment(ebs *apiebs.ResourceAttachment) error {
	var err error
	log.Info("Running HandleResourceAttachment")
	var wg sync.WaitGroup
	wg.Add(1)
	w.mailbox <- func() {
		defer wg.Done()
		empty := len(w.agentState.GetAllPendingEBSAttachments()) == 0

		log.Info("Handling EBS attachment")
		err := w.handleEBSAttachment(ebs)
		if err != nil {
			log.Info("Failed to handle resource attachment")
		}
		if empty && len(w.agentState.GetAllPendingEBSAttachments()) == 1 {
			w.scanTicker.Stop()
			w.scanTicker = time.NewTicker(scanPeriod)
			log.Info()
		}
	}
	log.Info("HandleResourceAttachment finished")
	wg.Wait()
	return err
}

func (w *EBSWatcher) handleEBSAttachment(ebs *apiebs.ResourceAttachment) error {
	if ebs.AttachmentProperties[apiebs.ResourceTypeName] != apiebs.ElasticBlockStorage {
		log.Info("Resource type not Elastic Block Storage. Skip handling resource attachment.")
		return nil
	}
	volumeID := ebs.AttachmentProperties[apiebs.VolumeIdName]
	_, ok := w.agentState.GetEBSByVolumeId(volumeID)
	log.Infof("Handling EBS attachment with volume ID: %v", volumeID)

	if ok {
		log.Info("EBS Volume attachment already exists. Skip handling EBS attachment.")
		return nil
	}

	if ebs.IsSent() {
		log.Info("Resource already attached. Skip handling EBS attachment.")
		return nil
	}

	duration := time.Until(ebs.ExpiresAt)
	if duration <= 0 {
		log.Info("Attachment expiration time has past. Skip handling EBS attachment")
		return nil
	}

	ebs.Initialize(func() {
		log.Info("EBS Volume timed out: %v", volumeID)
		w.RemoveAttachment(volumeID)
	})

	w.agentState.AddEBSAttachment(ebs)
	log.Info("EBS attachment added to state")
	return nil
}

func (w *EBSWatcher) notifyFoundEBS(volumeId string) {
	w.mailbox <- func() {
		ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
		if !ok {
			return
		}
		log.Infof("Found EBS volume with volumd ID: %v and device name: %v", volumeId, ebs.AttachmentProperties[apiebs.DeviceName])
		ebs.StopAckTimer()
		w.removeEBSAttachment(volumeId)
	}
}

func (w *EBSWatcher) RemoveAttachment(volumeID string) {
	w.mailbox <- func() {
		w.removeEBSAttachment(volumeID)
	}
}

func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	log.Info("Removing EBS volume")
	w.agentState.RemoveEBSAttachment(volumeID)
	log.Info("EBS attachment has been removed.")
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		log.Info("No more attachments to scan for. Stopping scan timer...")
		w.scanTicker.Stop()
	}
}

func (w *EBSWatcher) scanEBSVolumes() {
	for _, ebs := range w.agentState.GetAllPendingEBSAttachments() {
		volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
		deviceName := ebs.AttachmentProperties[apiebs.DeviceName]
		log.Infof("Scanning for EBS volume with volume ID: %v and device name: %v", volumeId, deviceName)
		err := apiebs.ConfirmEBSVolumeIsAttached(w.ctx, deviceName, volumeId)
		if err != nil {
			log.Infof("Unable to find EBS volume with volume ID: %v and device name: %v", volumeId, deviceName)
			if err == apiebs.ErrInvalidVolumeID || errors.Cause(err) == apiebs.ErrInvalidVolumeID {
				log.Info("Found a different EBS volume attached to the host")
				w.agentState.RemoveEBSAttachment(volumeId)
			}
			log.Infof("Error: %v", err)
			w.agentState.RemoveEBSAttachment(volumeId)
			continue
		}
		log.Info("EBS volume has been found")
		w.notifyFoundEBS(volumeId)
	}
}

// // Perhaps we need another struct called EBSHandler (similar to the existing ENIHandler)

// // This will be called during the HandleResourceAttachment and will essentially do what addENIAttachmentToState is doing but for resource attachments
// func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
// 	return nil
// }

// // This will remove the EBS attachment after the timeout period, will called when the EBS attachment starts it ACK timer (which will be in addEBSAttachmentToState)
// func (w *EBSWatcher) handleEBSAckTimeout(volumeId string) {

// }

// // Will be called by handleEBSAckTimeout
// func (w *EBSWatcher) removeEBSAttachmentData(volumeId string) {

// }
