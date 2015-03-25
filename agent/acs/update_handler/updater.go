// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package updater

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	acsclient "github.com/aws/amazon-ecs-agent/agent/acs/client"
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("updater")

const desiredImageFile = "desired-image"

// update describes metadata around an update 2-phase request
type updater struct {
	stage             updateStage
	downloadMessageID string
	fs                os.FileSystem
	acs               acsclient.ClientServer
	config            *config.Config
}

type updateStage int8

const (
	updateNone updateStage = iota
	updateDownloading
	updateDownloaded
)

// AddAgentUpdateHandlers adds the needed update handlers to perform agent
// updates
func AddAgentUpdateHandlers(cs acsclient.ClientServer, cfg *config.Config, saver statemanager.Saver, taskEngine engine.TaskEngine) {
	log.Debug("Adding update handlers")

	if cfg.UpdatesEnabled {
		acsUpdater := &updater{
			acs:    cs,
			config: cfg,
			fs:     os.Default,
		}
		cs.AddRequestHandler(acsUpdater.stageUpdateHandler())
		cs.AddRequestHandler(acsUpdater.performUpdateHandler(saver, taskEngine))
		log.Debug("Added update handlers")
	}
}

func (u *updater) stageUpdateHandler() func(req *ecsacs.StageUpdateMessage) {
	return func(req *ecsacs.StageUpdateMessage) {
		if req == nil || req.MessageId == nil {
			return
		}
		log.Debug("Staging update", "update", req)

		nack := func(reason string) {
			log.Debug("Nacking update", "reason", reason)
			u.stage = updateNone
			u.acs.MakeRequest(&ecsacs.NackRequest{
				Cluster:           req.ClusterArn,
				ContainerInstance: req.ContainerInstanceArn,
				MessageId:         req.MessageId,
				Reason:            &reason,
			})
		}

		if u.stage != updateNone {
			// update.cancel() // TODO

			// Cancel and nack previous update
			reason := "New update arrived: " + *req.MessageId
			u.acs.MakeRequest(&ecsacs.NackRequest{
				Cluster:           req.ClusterArn,
				ContainerInstance: req.ContainerInstanceArn,
				MessageId:         &u.downloadMessageID,
				Reason:            &reason,
			})
		}
		u.stage = updateDownloading
		u.downloadMessageID = *req.MessageId
		if req.UpdateInfo == nil || req.UpdateInfo.Location == nil {
			nack("Update location not set")
			return
		}

		err := u.download(req.UpdateInfo)
		if err != nil {
			nack("Unable to download: " + err.Error())
			return
		}

		u.stage = updateDownloaded

		u.acs.MakeRequest(&ecsacs.AckRequest{
			Cluster:           req.ClusterArn,
			ContainerInstance: req.ContainerInstanceArn,
			MessageId:         req.MessageId,
		})
	}
}

func (u *updater) download(info *ecsacs.UpdateInfo) error {
	if info == nil || info.Location == nil {
		return errors.New("No location given")
	}
	if info.Signature == nil {
		return errors.New("No signature given")
	}
	resp, err := httpclient.Default.Get(*info.Location)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}

	outFileBasename := utils.RandHex() + ".ecs-update.tar"
	outFile, err := u.fs.Create(filepath.Join(u.config.UpdateDownloadDir, outFileBasename))
	if err != nil {
		return err
	}

	hashsum := sha256.New()
	bodyHashReader := io.TeeReader(resp.Body, hashsum)
	_, err = io.Copy(outFile, bodyHashReader)
	if err != nil {
		return err
	}
	shasum := hashsum.Sum(nil)
	shasumString := fmt.Sprintf("%x", shasum)

	if shasumString != strings.TrimSpace(*info.Signature) {
		return errors.New("Hashsum validation failed")
	}

	err = u.fs.WriteFile(filepath.Join(u.config.UpdateDownloadDir, desiredImageFile), []byte(outFileBasename+"\n"), 0644)
	return err
}

func (u *updater) performUpdateHandler(saver statemanager.Saver, taskEngine engine.TaskEngine) func(req *ecsacs.PerformUpdateMessage) {
	return func(req *ecsacs.PerformUpdateMessage) {
		log.Debug("Got perform update request")
		if u.stage != updateDownloaded {
			log.Debug("Nacking update; not downloaded")
			reason := "Cannot perform update; update not downloaded"
			u.acs.MakeRequest(&ecsacs.NackRequest{
				Cluster:           req.ClusterArn,
				ContainerInstance: req.ContainerInstanceArn,
				MessageId:         &u.downloadMessageID,
				Reason:            &reason,
			})
		}

		taskEngine.Disable()
		var err error
		log.Debug("Saving state before shutting down for update")
		if forceSaver, ok := saver.(statemanager.ForceSaver); ok {
			err = forceSaver.ForceSave()
		} else {
			err = saver.Save()
		}
		if err != nil {
			log.Crit("Error saving state before final shutdown", "err", err)
		} else {
			log.Debug("Saved state!")
		}
		u.fs.Exit(42)
	}
}
