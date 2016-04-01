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
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/context"

	acsclient "github.com/aws/amazon-ecs-agent/agent/acs/client"
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	utilatomic "github.com/aws/amazon-ecs-agent/agent/utils/atomic"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

var log = logger.ForModule("acs handler")

// The maximum time to wait between heartbeats without disconnecting
const (
	heartbeatTimeout = 5 * time.Minute
	heartbeatJitter  = 3 * time.Minute

	connectionBackoffMin        = 250 * time.Millisecond
	connectionBackoffMax        = 2 * time.Minute
	connectionBackoffJitter     = 0.2
	connectionBackoffMultiplier = 1.5
)

// Maximum number of payload messages to queue up without having handled previous ones.
const payloadMessageBufferSize = 10

// SequenceNumber is a number shared between all ACS clients which indicates
// the last sequence number successfully handled.
var SequenceNumber = utilatomic.NewIncreasingInt64(1)

// StartSessionArguments is a struct representing all the things this handler
// needs... This is really a hack to get by-name instead of positional
// arguments since there are too many for positional to be wieldy
type StartSessionArguments struct {
	ContainerInstanceArn string
	CredentialProvider   *credentials.Credentials
	Config               *config.Config
	TaskEngine           engine.TaskEngine
	ECSClient            api.ECSClient
	StateManager         statemanager.StateManager
	AcceptInvalidCert    bool
}

// StartSession creates a session with ACS and handles requests using the passed
// in arguments.
func StartSession(ctx context.Context, args StartSessionArguments) error {
	ecsclient := args.ECSClient
	cfg := args.Config
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)

	payloadBuffer := make(chan *ecsacs.PayloadMessage, payloadMessageBufferSize)
	ackBuffer := make(chan string, payloadMessageBufferSize)

	// Handle any payloads async.
	go payloadBufferHandler(payloadBuffer, ackBuffer, args.TaskEngine, ecsclient, args.StateManager, ctx)

	for {
		acsError := func() error {
			acsEndpoint, err := ecsclient.DiscoverPollEndpoint(args.ContainerInstanceArn)
			if err != nil {
				log.Error("Unable to discover poll endpoint", "err", err)
				return err
			}
			log.Debug("Connecting to ACS endpoint " + acsEndpoint)

			url := AcsWsUrl(acsEndpoint, cfg.Cluster, args.ContainerInstanceArn, args.TaskEngine)

			clearStrChannel(ackBuffer)
			client := acsclient.New(url, cfg.AWSRegion, args.CredentialProvider, args.AcceptInvalidCert)
			defer client.Close()
			// Clear the ackbuffer whenever we get a new client because acks of
			// messageids don't have any value across sessions
			defer clearStrChannel(ackBuffer)

			timer := ttime.AfterFunc(utils.AddJitter(heartbeatTimeout, heartbeatJitter), func() {
				log.Warn("ACS Connection hasn't had any activity for too long; closing connection")
				closeErr := client.Close()
				if closeErr != nil {
					log.Warn("Error disconnecting: " + closeErr.Error())
				}
			})
			defer timer.Stop()
			// Any message from the server resets the disconnect timeout
			client.SetAnyRequestHandler(anyMessageHandler(timer))
			client.AddRequestHandler(payloadMessageHandler(payloadBuffer))
			// Ignore heartbeat messages; anyMessageHandler gets 'em
			client.AddRequestHandler(func(*ecsacs.HeartbeatMessage) {})

			updater.AddAgentUpdateHandlers(client, cfg, args.StateManager, args.TaskEngine)

			err = client.Connect()
			if err != nil {
				log.Error("Error connecting to ACS: " + err.Error())
				return err
			}
			ttime.AfterFunc(utils.AddJitter(heartbeatTimeout, heartbeatJitter), func() {
				// If we do not have an error connecting and remain connected for at
				// least 5 or so minutes, reset the backoff. This prevents disconnect
				// errors that only happen infrequently from damaging the
				// reconnectability as significantly.
				backoff.Reset()
			})

			serveErr := make(chan error, 1)
			go func() {
				serveErr <- client.Serve()
			}()

			for {
				select {
				case mid := <-ackBuffer:
					ackMessageId(client, cfg.Cluster, args.ContainerInstanceArn, mid)
				case <-ctx.Done():
					return ctx.Err()
				case err := <-serveErr:
					return err
				}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if acsError == nil || acsError == io.EOF {
			backoff.Reset()
		} else {
			log.Info("Error from acs; backing off", "err", acsError)
			ttime.Sleep(backoff.Duration())
		}
	}
}

// payloadBufferHandler processes payload messages from the payload buffer.
// For correctness, they must be handled in order, hence the buffered channel which is added to synchronously.
func payloadBufferHandler(payloadBuffer <-chan *ecsacs.PayloadMessage, responseChan chan<- string, taskEngine engine.TaskEngine, ecsClient api.ECSClient, saver statemanager.Saver, ctx context.Context) {
	for {
		select {
		case payload := <-payloadBuffer:
			handlePayloadMessage(responseChan, payload, taskEngine, ecsClient, saver)
		case <-ctx.Done():
			return
		}
	}
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active and thus the heartbeat disconnect should not occur
func anyMessageHandler(timer ttime.Timer) func(interface{}) {
	return func(interface{}) {
		log.Debug("ACS activity occured")
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// payloadMessageHandler returns a handler for payload messages which
// takes given payloads, converts them into the internal representation of
// tasks, and passes them on to the task engine. If there is an issue handling a
// task, it is moved to stopped. If a task is handled, state is saved.
func payloadMessageHandler(messageBuffer chan<- *ecsacs.PayloadMessage) func(payload *ecsacs.PayloadMessage) {
	return func(payload *ecsacs.PayloadMessage) {
		messageBuffer <- payload
	}
}

// handlePayloadMessage attempts to add each task to the taskengine and, if it can, acks the request.
// It returns an error if the message can not be ack'd for any reason. The error code is being used
// only for testing today. In the future, it could be used for doing more interesting things.
func handlePayloadMessage(responseChan chan<- string, payload *ecsacs.PayloadMessage, taskEngine engine.TaskEngine, client api.ECSClient, saver statemanager.Saver) error {
	if aws.StringValue(payload.MessageId) == "" {
		log.Crit("Recieved a payload with no message id", "payload", payload)
		return fmt.Errorf("Received a payload with no message id")
	}
	allTasksHandled := addPayloadTasks(client, payload, taskEngine)
	// save the state of tasks we know about after passing them to the task engine
	err := saver.Save()
	if err != nil {
		log.Error("Error saving state for payload message!", "err", err, "messageId", *payload.MessageId)
		// Don't ack; maybe we can save it in the future.
		return fmt.Errorf("Error saving state for payload message, with messageId: %s", *payload.MessageId)
	}
	if !allTasksHandled {
		return fmt.Errorf("All tasks not handled")
	}

	go func() {
		// Throw the ack in async; it doesn't really matter all that much and this is blocking handling more tasks.
		responseChan <- *payload.MessageId
	}()
	// Record the sequence number as well
	if payload.SeqNum != nil {
		SequenceNumber.Set(*payload.SeqNum)
	}
	return nil
}

// addPayloadTasks does validation on each task and, for all valid ones, adds
// it to the task engine. It returns a bool indicating if it could add every
// task to the taskEngine
func addPayloadTasks(client api.ECSClient, payload *ecsacs.PayloadMessage, taskEngine engine.TaskEngine) bool {
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
			handleUnrecognizedTask(client, task, err, payload)
			allTasksOk = false
			continue
		}
		validTasks = append(validTasks, apiTask)
	}
	// Add 'stop' transitions first to allow seqnum ordering to work out
	// Because a 'start' sequence number should only be proceeded if all 'stop's
	// of the same sequence number have completed, the 'start' events need to be
	// added after the 'stop' events are there to block them.
	stoppedAddedOk := addStoppedTasks(validTasks, taskEngine)
	nonstoppedAddedOk := addNonstoppedTasks(validTasks, taskEngine)
	if !stoppedAddedOk || !nonstoppedAddedOk {
		allTasksOk = false
	}
	return allTasksOk
}

func addStoppedTasks(tasks []*api.Task, taskEngine engine.TaskEngine) bool {
	allTasksOk := true
	for _, task := range tasks {
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
	return allTasksOk
}

func addNonstoppedTasks(tasks []*api.Task, taskEngine engine.TaskEngine) bool {
	allTasksOk := true
	for _, task := range tasks {
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

// handleUnrecognizedTask handles unrecognized tasks by sending 'stopped' with
// a suitable reason to the backend
func handleUnrecognizedTask(client api.ECSClient, task *ecsacs.Task, err error, payload *ecsacs.PayloadMessage) {
	if task.Arn == nil {
		log.Crit("Recieved task with no arn", "task", task, "messageId", *payload.MessageId)
		return
	}

	// Only need to stop the task; it brings down the containers too.
	eventhandler.AddTaskEvent(api.TaskStateChange{
		TaskArn: *task.Arn,
		Status:  api.TaskStopped,
		Reason:  UnrecognizedTaskError{err}.Error(),
	}, client)
}

func clearStrChannel(c <-chan string) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func ackMessageId(cs wsclient.ClientServer, cluster, containerInstanceArn, messageID string) {
	err := cs.MakeRequest(&ecsacs.AckRequest{
		Cluster:           &cluster,
		ContainerInstance: &containerInstanceArn,
		MessageId:         &messageID,
	})
	if err != nil {
		log.Warn("Error 'ack'ing request", "MessageID", messageID)
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
	query.Set("seqNum", strconv.FormatInt(SequenceNumber.Get(), 10))
	if dockerVersion, err := taskEngine.Version(); err == nil {
		query.Set("dockerVersion", dockerVersion)
	}
	return acsUrl + "?" + query.Encode()
}
