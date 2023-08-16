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
	scanPeriod = 500 * time.Millisecond
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	scanTicker *time.Ticker
	agentState dockerstate.TaskEngineState
	// TODO: The dataClient will be used to save to agent's data client as well as start the ACK timer. This will be added once the data client functionality have been added
	// dataClient     data.Client
	ebsChangeEvent  chan<- statechange.Event
	discoveryClient apiebs.EBSDiscovery
	mailbox         chan func()
}

func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*EBSWatcher, error) {
	derivedContext, cancel := context.WithCancel(ctx)
	discoveryClient := apiebs.NewDiscoveryClient(derivedContext)
	log.Info("EBS watcher has been initialized")
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      state,
		ebsChangeEvent:  stateChangeEvents,
		discoveryClient: discoveryClient,
		mailbox:         make(chan func(), 100),
	}, nil
}

func (w *EBSWatcher) Start() {
	log.Info("Starting Linux EBS watcher")

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
	log.Info("Running HandleResourceAttachment")
	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	w.mailbox <- func() {
		defer wg.Done()
		empty := len(w.agentState.GetAllPendingEBSAttachments()) == 0

		// log.Info("Handling EBS attachment")
		err := w.handleEBSAttachment(ebs)
		if err != nil {
			log.Info("Failed to handle resource attachment")
		}
		if empty && len(w.agentState.GetAllPendingEBSAttachments()) == 1 {
			log.Info("Restarting the scan ticker...")
			w.scanTicker.Stop()
			w.scanTicker = time.NewTicker(scanPeriod)
			log.Info()
		}
		time.Sleep(time.Millisecond)
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
	ebsAttachment, ok := w.agentState.GetEBSByVolumeId(volumeID)
	log.Infof("Handling EBS attachment with volume ID: %v", volumeID)

	// If there is already an EBS attachment, start its ACK timer if it hasn't done so
	if ok {
		log.Info("EBS Volume attachment already exists. Skip handling EBS attachment.")

		// In the current containerd agent process, the timer does not reset if we get another
		return ebsAttachment.StartTimer(func() {
			log.Infof("ACK timer expired for EBS volume: %v", volumeID)
			w.handleEBSAckTimeout(volumeID)
		})
	}

	if err := w.addEBSAttachmentToState(ebs); err != nil {
		return err
	}
	log.Info("EBS attachment added to state")
	return nil
}

func (w *EBSWatcher) notifyFoundEBS(volumeId string) {
	w.mailbox <- func() {
		ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
		if !ok {
			log.Infof("No EBS volume with volume ID: %v", volumeId)
			return
		}
		log.Infof("Found EBS volume with volume ID: %v and device name: %v", volumeId, ebs.AttachmentProperties[apiebs.DeviceName])
		ebs.StopAckTimer()
		// Would have sent a State change event
		log.Info("Would have sent a state change event for EBS attachment, setting the sent status to true...")
		ebs.SetSentStatus()
		if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
			log.Info("No more attachments to scan for. Stopping scan ticker...")
			w.scanTicker.Stop()
		}
		// w.removeEBSAttachment(volumeId)
		time.Sleep(time.Millisecond)
	}
}

func (w *EBSWatcher) RemoveAttachment(volumeID string) {
	w.mailbox <- func() {
		w.removeEBSAttachment(volumeID)
		time.Sleep(time.Millisecond)
	}
}

func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	log.Info("Removing EBS volume")

	log.Infof("Would have removed EBS volume: %v, from data client", volumeID)
	// attachmentToRemove, ok := w.agentState.GetEBSByVolumeId(volumeID)
	// if !ok {
	// 	log.Errorf("Unable to retrieve EBS attachment for volume ID: %s", volumeID)
	// }
	// attachmentId, err := utils.GetEBSAttachmentId(attachmentToRemove.AttachmentARN)
	// if err != nil {
	// 	log.Errorf("Failed to get attachment id for %s, Error: %v", attachmentToRemove.AttachmentARN, err)
	// } else {
	// 	err = w.dataClient.DeleteEBSAttachment(attachmentId)
	// 	if err != nil {
	// 		log.Errorf("Failed to remove data for ebs attachment %s, Error: %v", attachmentId, err)
	// 	}
	// }

	w.agentState.RemoveEBSAttachment(volumeID)
	log.Infof("EBS attachment with volume ID: %v has been removed from agent state.", volumeID)
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		log.Info("No more attachments to scan for. Stopping scan ticker...")
		w.scanTicker.Stop()
	}
}

func (w *EBSWatcher) scanEBSVolumes() {
	log.Infof("Scanning for EBS volumes...")
	for _, ebs := range w.agentState.GetAllPendingEBSAttachments() {
		volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
		deviceName := ebs.AttachmentProperties[apiebs.DeviceName]
		log.Infof("Scanning for EBS volume with volume ID: %v and device name: %v", volumeId, deviceName)
		// err := apiebs.ConfirmEBSVolumeIsAttached(w.ctx, deviceName, volumeId)
		err := w.discoveryClient.ConfirmEBSVolumeIsAttached(deviceName, volumeId)
		if err != nil {
			log.Infof("Unable to find EBS volume with volume ID: %v and device name: %v", volumeId, deviceName)
			if err == apiebs.ErrInvalidVolumeID || errors.Cause(err) == apiebs.ErrInvalidVolumeID {
				log.Info("Found a different EBS volume attached to the host")
			} else {
				log.Infof("Error: %v", err)
			}
			continue
		}
		log.Infof("EBS volume with id: %v, has been found", volumeId)
		w.notifyFoundEBS(volumeId)
	}
}

// // Perhaps we need another struct called EBSHandler (similar to the existing ENIHandler)

// This will be called during the HandleResourceAttachment and will essentially do what addENIAttachmentToState is doing but for resource attachments
func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
	// attachmentARN := ebs.AttachmentARN
	volumeId := string(ebs.AttachmentProperties[apiebs.VolumeIdName])
	err := ebs.StartTimer(func() {
		log.Infof("ACK timer expired for EBS volume: %v", volumeId)
		w.handleEBSAckTimeout(volumeId)
	})
	if err != nil {
		return err
	}
	log.Infof("Adding ebs attachment info to state, volume ID: %s", volumeId)

	w.agentState.AddEBSAttachment(ebs)

	log.Info("Would have added EBS to data client")
	// if err := w.dataClient.SaveEBSAttachment(ebs); err != nil {
	// 	log.Errorf("Failed to save data for ebs attachment with error: %v", err)
	//  return err
	// }

	return nil
}

// // This will remove the EBS attachment after the timeout period, will called when the EBS attachment starts it ACK timer (which will be in addEBSAttachmentToState)
func (w *EBSWatcher) handleEBSAckTimeout(volumeId string) {
	log.Infof("Handling expired EBS attachment: %v", volumeId)
	ebsAttachment, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Ignoring unmanaged EBS attachment volume ID=%s", volumeId)
		return
	}
	if !ebsAttachment.IsSent() {
		log.Warnf("Timed out waiting for EBS ack; removing EBS attachment record %v", ebsAttachment)
		w.RemoveAttachment(volumeId)
	}
}
