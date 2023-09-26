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
	"strconv"
	"time"

	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	csi "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	log "github.com/cihub/seelog"

	v1 "k8s.io/api/core/v1"
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	agentState dockerstate.TaskEngineState
	// TODO: The dataClient will be used to save to agent's data client as well as start the ACK timer. This will be added once the data client functionality have been added
	// dataClient     data.Client
	discoveryClient apiebs.EBSDiscovery
	csiClient       csi.CSIClient
	scanTicker      *time.Ticker
	// TODO: The dockerTaskEngine.stateChangeEvent will be used to send over the state change event for EBS attachments once it's been found and mounted/resize/format.
	taskEngine ecsengine.TaskEngine
}

// NewWatcher is used to return a new instance of the EBSWatcher struct
func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	taskEngine ecsengine.TaskEngine) *EBSWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	discoveryClient := apiebs.NewDiscoveryClient(derivedContext)
	// TODO pull this socket out into config
	csiClient := csi.NewCSIClient("/var/run/ecs/ebs-csi-driver/csi-driver.sock")
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      state,
		discoveryClient: discoveryClient,
		csiClient:       &csiClient,
		taskEngine:      taskEngine,
	}
}

// Start is used to kick off the periodic scanning process of the EBS volume attachments for the EBS watcher.
// It will be start and continue to run whenever there's a pending EBS volume attachment that hasn't been found.
// If there aren't any, the scan ticker will not start up/scan for volumes.
func (w *EBSWatcher) Start() {
	log.Info("Starting EBS watcher.")
	w.scanTicker = time.NewTicker(apiebs.ScanPeriod)
	for {
		select {
		case <-w.scanTicker.C:
			pendingEBS := w.agentState.GetAllPendingEBSAttachmentsWithKey()
			if len(pendingEBS) > 0 {
				foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, w.discoveryClient)
				if err := w.StageAll(foundVolumes); err != nil {
					w.NotifyAttached(foundVolumes)
				}
			}
		case <-w.ctx.Done():
			w.scanTicker.Stop()
			log.Info("EBS Watcher Stopped due to agent stop")
			return
		}
	}
}

// Stop will stop the EBS watcher
func (w *EBSWatcher) Stop() {
	log.Info("Stopping EBS watcher.")
	w.cancel()
}

// HandleResourceAttachment processes the resource attachment message. It will:
// 1. Check whether we already have this attachment in state and if so it's a noop.
// 2. Start the EBS CSI driver if it's not already running
// 3. Otherwise add the attachment to state, start its ack timer, and save to the agent state.
func (w *EBSWatcher) HandleResourceAttachment(ebs *apiebs.ResourceAttachment) error {
	attachmentType := ebs.GetAttachmentProperties(apiebs.ResourceTypeName)
	if attachmentType != apiebs.ElasticBlockStorage {
		log.Warnf("Resource type not Elastic Block Storage. Skip handling resource attachment with type: %v.", attachmentType)
		return nil
	}
	volumeId := ebs.GetAttachmentProperties(apiebs.VolumeIdName)
	ebsAttachment, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if ok {
		log.Infof("EBS Volume attachment already exists. Skip handling EBS attachment %v.", ebs.EBSToString())
		return ebsAttachment.StartTimer(func() {
			w.handleEBSAckTimeout(volumeId)
		})
	}

	// start EBS CSI Driver Managed Daemon
	if w.taskEngine.GetEbsDaemonTask() != nil {
		log.Infof("engine ebs CSI driver is running with taskID: %v", w.taskEngine.GetEbsDaemonTask().GetID())
	} else {
		if ebsCsiDaemonManager, ok := w.taskEngine.GetDaemonManagers()["ebs-csi-driver"]; ok {
			if csiTask, err := ebsCsiDaemonManager.CreateDaemonTask(); err != nil {
				// fail attachment and return
				log.Errorf("Unable to start ebsCsiDaemon for task %s in the engine: error: %s", csiTask.GetID(), err)
				csiTask.SetKnownStatus(apitaskstatus.TaskStopped)
				csiTask.SetDesiredStatus(apitaskstatus.TaskStopped)
				w.taskEngine.EmitTaskEvent(csiTask, err.Error())
				return err
			} else {
				w.taskEngine.SetEbsDaemonTask(csiTask)
				w.taskEngine.AddTask(csiTask)
				log.Infof("task_engine: Added EBS CSI task to engine")
			}
		} else {
			log.Errorf("CSI Driver is not Initialized")
		}
	}

	if err := w.addEBSAttachmentToState(ebs); err != nil {
		return fmt.Errorf("%w; attach %v message handler: unable to add ebs attachment to engine state: %v",
			err, attachmentType, ebs.EBSToString())
	}

	return nil
}

func (w *EBSWatcher) StageAll(foundVolumes []string) error {
	for _, thisVol := range foundVolumes {
		// assumes CSI Driver Managed Daemon is running else call will timeout
		log.Infof("CSI Staging volume: %s", thisVol)

		// get volume details from attachment
		ebsAttachment, _ := w.agentState.GetEBSByVolumeId(thisVol)
		deviceName := ebsAttachment.GetAttachmentProperties(apiebs.DeviceNameKey)
		hostPath := ebsAttachment.GetAttachmentProperties(apiebs.SourceVolumeHostPathKey)
		filesystemType := ebsAttachment.GetAttachmentProperties(apiebs.FileSystemTypeName)

		// CSI NodeStage stub equired fields
		stubSecrets := make(map[string]string)
		stubVolumeContext := make(map[string]string)
		stubMountOptions := []string{}
		stubFsGroup, _ := strconv.ParseInt("123456", 10, 8)

		publishContext := map[string]string{"devicePath": deviceName}
		// call CSI NodeStage
		err := w.csiClient.NodeStageVolume(w.ctx,
			thisVol,
			publishContext,
			hostPath,
			filesystemType,
			v1.ReadWriteMany,
			stubSecrets,
			stubVolumeContext,
			stubMountOptions,
			&stubFsGroup)
		if err != nil {
			log.Errorf("Failed to initialize EBS volume: error: %s", err)
			return err
		}
	}
	return nil
}

// NotifyAttached will go through the list of found EBS volumes from the scanning process and mark them as found.
func (w *EBSWatcher) NotifyAttached(foundVolumes []string) {
	for _, volumeId := range foundVolumes {
		w.notifyAttachedEBS(volumeId)
	}
}

// notifyAttachedEBS will mark it as found within the agent state
func (w *EBSWatcher) notifyAttachedEBS(volumeId string) {
	// TODO: Add the EBS volume to data client
	ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Unable to find EBS volume with volume ID: %v within agent state.", volumeId)
		return
	}

	if ebs.HasExpired() {
		log.Warnf("EBS status expired, no longer tracking EBS volume: %v.", ebs.EBSToString())
		return
	}

	if ebs.IsSent() {
		log.Warnf("State change event has already been emitted for EBS volume: %v.", ebs.EBSToString())
		return
	}

	if ebs.IsAttached() {
		log.Infof("EBS volume: %v, has been found already.", ebs.EBSToString())
		return
	}

	ebs.StopAckTimer()
	ebs.SetAttachedStatus()

	log.Infof("Successfully found attached EBS volume: %v", ebs.EBSToString())
}

// removeEBSAttachment removes a EBS volume with a specific volume ID
func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	// TODO: Remove the EBS volume from the data client.
	w.agentState.RemoveEBSAttachment(volumeID)
}

// addEBSAttachmentToState adds an EBS attachment to state, and start its ack timer
func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
	volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
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
		log.Warnf("Ignoring unmanaged EBS attachment volume ID=%v", volumeId)
		return
	}
	if !ebsAttachment.IsSent() {
		log.Warnf("Timed out waiting for EBS ack; removing EBS attachment record %v", ebsAttachment.EBSToString())
		w.removeEBSAttachment(volumeId)
	}
}
