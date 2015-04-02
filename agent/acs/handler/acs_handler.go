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

// Package handler deals with appropriately reacting to all ACS messages as well
// as maintaining the connection to ACS.
package handler

import (
	"io"
	"net/url"
	"time"

	acsclient "github.com/aws/amazon-ecs-agent/agent/acs/client"
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/version"
)

var log = logger.ForModule("acs handler")

// The maximum time to wait between heartbeats without disconnecting
const heartbeatTimeout = 5 * time.Minute
const heartbeatJitter = 3 * time.Minute

// StartSession creates a session with ACS and handles requests using the passed
// in arguments.
func StartSession(containerInstanceArn string, credentialProvider credentials.AWSCredentialProvider, cfg *config.Config, taskEngine engine.TaskEngine, ecsclient api.ECSClient, stateManager statemanager.StateManager, acceptInvalidCert bool) error {
	backoff := utils.NewSimpleBackoff(time.Second, 1*time.Minute, 0.2, 2)
	return utils.RetryWithBackoff(backoff, func() error {
		acsEndpoint, err := ecsclient.DiscoverPollEndpoint(containerInstanceArn)
		if err != nil {
			log.Error("Unable to discover poll endpoint", "err", err)
			return err
		}

		url := AcsWsUrl(acsEndpoint, cfg.Cluster, containerInstanceArn, taskEngine)

		client := acsclient.New(url, cfg.AWSRegion, credentialProvider, acceptInvalidCert)

		client.AddRequestHandler(payloadMessageHandler(client, cfg.Cluster, containerInstanceArn, taskEngine, ecsclient, stateManager))
		client.AddRequestHandler(heartbeatHandler(client))

		updater.AddAgentUpdateHandlers(client, cfg, stateManager, taskEngine)

		err = client.Connect()
		if err != nil {
			log.Error("Error connecting to ACS: " + err.Error())
			return err
		}
		return client.Serve()
	})
}

// heartbeatHandler starts a timer and listens for acs heartbeats. If there are
// none for unexpectedly long, it closes the passed in connection.
func heartbeatHandler(acsConnection io.Closer) func(*ecsacs.HeartbeatMessage) {
	timer := time.AfterFunc(utils.AddJitter(heartbeatTimeout, heartbeatJitter), func() {
		acsConnection.Close()
	})
	return func(*ecsacs.HeartbeatMessage) {
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// payloadMessageHandler returns a handler for payload messages which
// takes given payloads, converts them into the internal representation of
// tasks, and passes them on to the task engine. If there is an issue handling a
// task, it is moved to stopped. If a task is handled, state is saved.
func payloadMessageHandler(cs acsclient.ClientServer, cluster, containerInstanceArn string, taskEngine engine.TaskEngine, client api.ECSClient, stateManager statemanager.Saver) func(payload *ecsacs.PayloadMessage) {
	return func(payload *ecsacs.PayloadMessage) {
		for _, task := range payload.Tasks {
			if task == nil {
				log.Error("Recieved nil task")
				return
			}
			apiTask, err := api.TaskFromACS(task)
			if err != nil {
				if task.Arn == nil {
					log.Error("Recieved task with no arn", "task", task)
					return
				}
				// If there was an error converting these from acs to engine
				// tasks, report to the backend that they're not running and
				// give a suitable reason
				badtaskChanges := make([]*api.ContainerStateChange, 0, len(task.Containers))
				for _, container := range task.Containers {
					if container == nil {
						log.Error("Recieved task with nil containers", "arn", *task.Arn)
						continue
					}
					if container.Name == nil {
						log.Error("Recieved task with nil container name", "arn", *task.Arn)
						continue
					}
					change := api.ContainerStateChange{
						TaskArn:       *task.Arn,
						Status:        api.ContainerStopped,
						Reason:        "Error loading task: " + err.Error(),
						Container:     &api.Container{},
						ContainerName: *container.Name,
					}
					badtaskChanges = append(badtaskChanges, &change)
				}
				if len(badtaskChanges) == 0 {
					return
				}
				// The last container stop also brings down the task
				taskChange := badtaskChanges[len(badtaskChanges)-1]
				taskChange.TaskStatus = api.TaskStopped
				taskChange.Task = &api.Task{}

				for _, change := range badtaskChanges {
					eventhandler.AddTaskEvent(*change, client)
				}
				return
			}
			// Else, no error converting, add to engine
			err = taskEngine.AddTask(apiTask)
			if err != nil {
				log.Warn("Could not add task; taskengine probably disabled")
				// Don't ack
				return
			}
			err = stateManager.Save()
			if err != nil {
				log.Error("Error saving state!", "err", err)
				// Don't ack; maybe we can save it in the future.
				return
			}
			err = cs.MakeRequest(&ecsacs.AckRequest{
				Cluster:           &cluster,
				ContainerInstance: &containerInstanceArn,
				MessageId:         payload.MessageId,
			})
			if err != nil {
				mid := "null"
				if payload.MessageId != nil {
					mid = *payload.MessageId
				}
				log.Warn("Error 'ack'ing request", "MessageID", mid)
			}
		}
	}
}

// AcsWsUrl returns the websocket url for ACS given the endpoint.
func AcsWsUrl(endpoint, cluster, containerInstanceArn string, taskEngine engine.TaskEngine) string {
	acsUrl := endpoint
	if endpoint[len(endpoint)-1] != '/' {
		acsUrl += "/"
	}
	acsUrl += "ws"
	query := url.Values{}
	query.Set("clusterArn", cluster)
	query.Set("containerInstanceArn", containerInstanceArn)
	query.Set("agentHash", version.GitHashString())
	query.Set("agentVersion", version.Version)
	if dockerVersion, err := taskEngine.Version(); err == nil {
		query.Set("dockerVersion", dockerVersion)
	}
	return acsUrl + "?" + query.Encode()
}
