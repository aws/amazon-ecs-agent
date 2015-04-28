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
	"strconv"
	"sync/atomic"
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

// Maximum number of payload messages to queue up without having handled previous ones.
const payloadMessageBufferSize = 10

// SequenceNumber is a number shared between all ACS clients which indicates
// the last sequence number successfully handled.
var SequenceNumber int64 = 1

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
		defer client.Close()

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
	messageBuffer := make(chan *ecsacs.PayloadMessage, payloadMessageBufferSize)
	go func() {
		for message := range messageBuffer {
			handlePayloadMessage(cs, cluster, containerInstanceArn, message, taskEngine, client, stateManager)
		}
	}()

	return func(payload *ecsacs.PayloadMessage) {
		messageBuffer <- payload
	}
}

// handlePayloadMessage attempts to add each task to the taskengine and, if it can, acks the request.
func handlePayloadMessage(cs acsclient.ClientServer, cluster, containerInstanceArn string, payload *ecsacs.PayloadMessage, taskEngine engine.TaskEngine, client api.ECSClient, saver statemanager.Saver) {
	if payload.MessageId == nil {
		log.Crit("Recieved a payload with no message id", "payload", payload)
		return
	}
	err := saver.Save()
	if err != nil {
		log.Error("Error saving state for payload message!", "err", err, "messageId", *payload.MessageId)
		// Don't ack; maybe we can save it in the future.
		return
	}
	allTasksHandled := addPayloadTasks(cs, client, cluster, containerInstanceArn, payload, taskEngine)
	if allTasksHandled {
		err = cs.MakeRequest(&ecsacs.AckRequest{
			Cluster:           &cluster,
			ContainerInstance: &containerInstanceArn,
			MessageId:         payload.MessageId,
		})
		if err != nil {
			log.Warn("Error 'ack'ing request", "MessageID", *payload.MessageId)
		}
		// Record the sequence number as well
		if payload.SeqNum != nil {
			for *payload.SeqNum < SequenceNumber && !atomic.CompareAndSwapInt64(&SequenceNumber, SequenceNumber, *payload.SeqNum) {
				log.Debug("Atomic setting of sequence number failed. Unusual, but not critical")
			}
		}
	}
}

// addPayloadTasks does validation on each task and, for all valid ones, adds
// it to the task engine. It returns a bool indicating if it could add every
// task to the taskEngine
func addPayloadTasks(cs acsclient.ClientServer, client api.ECSClient, cluster, containerInstanceArn string, payload *ecsacs.PayloadMessage, taskEngine engine.TaskEngine) bool {
	// verify thatwe were able to work with all tasks in this payload so we know whether to ack the whole thing or not
	allTasksOk := true

	validTasks := make([]*api.Task, 0, len(payload.Tasks))
	for _, task := range payload.Tasks {
		if task == nil {
			log.Crit("Recieved nil task", "messageId", *payload.MessageId)
			allTasksOk = false
			continue
		}
		apiTask, err := api.TaskFromACS(task, payload)
		if err != nil {
			handleUnrecognizedTask(cs, client, cluster, containerInstanceArn, task, err, payload)
			allTasksOk = false
			continue
		}
		validTasks = append(validTasks, apiTask)
	}
	// Add 'stop' transitions first to allow seqnum ordering to work out
	for _, task := range validTasks {
		if task.DesiredStatus != api.TaskStopped {
			continue
		}
		err := taskEngine.AddTask(task)
		if err != nil {
			log.Warn("Could not add task; taskengine probably disabled")
			// Don't ack
			allTasksOk = false
		}
	}
	// Now add the rest of the tasks
	for _, task := range validTasks {
		if task.DesiredStatus == api.TaskStopped {
			continue
		}
		err := taskEngine.AddTask(task)
		if err != nil {
			log.Warn("Could not add task; taskengine probably disabled")
			// Don't ack
			allTasksOk = false
		}
	}
	return allTasksOk
}

// Unrecognized tasks are handled by sending 'stopped' with a suitable reason to the backend
func handleUnrecognizedTask(cs acsclient.ClientServer, client api.ECSClient, cluster, containerInstanceArn string, task *ecsacs.Task, err error, payload *ecsacs.PayloadMessage) {
	if task.Arn == nil {
		log.Crit("Recieved task with no arn", "task", task, "messageId", *payload.MessageId)
		return
	}

	// Only need to stop the task; it brings down the containers too.
	eventhandler.AddTaskEvent(api.TaskStateChange{
		TaskArn: *task.Arn,
		Status:  api.TaskStopped,
		Reason:  "UnrecognizedACSTask: Error loading task: " + err.Error(),
	}, client)
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
	query.Set("seqNum", strconv.FormatInt(SequenceNumber, 10))
	if dockerVersion, err := taskEngine.Version(); err == nil {
		query.Set("dockerVersion", dockerVersion)
	}
	return acsUrl + "?" + query.Encode()
}
