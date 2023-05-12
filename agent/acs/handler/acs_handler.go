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

// Package handler deals with appropriately reacting to all ACS messages as well
// as maintaining the connection to ACS.
package handler

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	updater "github.com/aws/amazon-ecs-agent/agent/acs/update_handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/version"
	rolecredentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	// heartbeatTimeout is the maximum time to wait between heartbeats
	// without disconnecting
	heartbeatTimeout = 1 * time.Minute
	heartbeatJitter  = 1 * time.Minute
	// wsRWTimeout is the duration of read and write deadline for the
	// websocket connection
	wsRWTimeout = 2*heartbeatTimeout + heartbeatJitter

	inactiveInstanceReconnectDelay = 1 * time.Hour

	// connectionTime is the maximum time after which agent closes its connection to ACS
	connectionTime   = 15 * time.Minute
	connectionJitter = 30 * time.Minute

	connectionBackoffMin        = 250 * time.Millisecond
	connectionBackoffMax        = 2 * time.Minute
	connectionBackoffJitter     = 0.2
	connectionBackoffMultiplier = 1.5
	// payloadMessageBufferSize is the maximum number of payload messages
	// to queue up without having handled previous ones.
	payloadMessageBufferSize = 10
	// sendCredentialsURLParameterName is the name of the URL parameter
	// in the ACS URL that is used to indicate if ACS should send
	// credentials for all tasks on establishing the connection
	sendCredentialsURLParameterName = "sendCredentials"
	inactiveInstanceExceptionPrefix = "InactiveInstanceException:"
	// ACS protocol version spec:
	// 1: default protocol version
	// 2: ACS will proactively close the connection when heartbeat acks are missing
	acsProtocolVersion = 2
	// numOfHandlersSendingAcks is the number of handlers that send acks back to ACS and that are not saved across
	// sessions. We use this to send pending acks, before agent initiates a disconnect to ACS.
	// they are: refreshCredentialsHandler, taskManifestHandler, and payloadHandler
	numOfHandlersSendingAcks = 3
)

// Session defines an interface for handler's long-lived connection with ACS.
type Session interface {
	Start() error
}

// session encapsulates all arguments needed by the handler to connect to ACS
// and to handle messages received by ACS. The Session.Start() method can be used
// to start processing messages from ACS.
type session struct {
	containerInstanceARN            string
	credentialsProvider             *credentials.Credentials
	agentConfig                     *config.Config
	deregisterInstanceEventStream   *eventstream.EventStream
	taskEngine                      engine.TaskEngine
	dockerClient                    dockerapi.DockerClient
	ecsClient                       api.ECSClient
	state                           dockerstate.TaskEngineState
	dataClient                      data.Client
	credentialsManager              rolecredentials.Manager
	taskHandler                     *eventhandler.TaskHandler
	ctx                             context.Context
	cancel                          context.CancelFunc
	backoff                         retry.Backoff
	clientFactory                   wsclient.ClientFactory
	sendCredentials                 bool
	latestSeqNumTaskManifest        *int64
	doctor                          *doctor.Doctor
	_heartbeatTimeout               time.Duration
	_heartbeatJitter                time.Duration
	connectionTime                  time.Duration
	connectionJitter                time.Duration
	_inactiveInstanceReconnectDelay time.Duration
}

// NewSession creates a new Session object
func NewSession(
	ctx context.Context,
	config *config.Config,
	deregisterInstanceEventStream *eventstream.EventStream,
	containerInstanceARN string,
	credentialsProvider *credentials.Credentials,
	dockerClient dockerapi.DockerClient,
	ecsClient api.ECSClient,
	taskEngineState dockerstate.TaskEngineState,
	dataClient data.Client,
	taskEngine engine.TaskEngine,
	credentialsManager rolecredentials.Manager,
	taskHandler *eventhandler.TaskHandler,
	latestSeqNumTaskManifest *int64,
	doctor *doctor.Doctor,
	clientFactory wsclient.ClientFactory,
) Session {
	backoff := retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
		connectionBackoffJitter, connectionBackoffMultiplier)
	derivedContext, cancel := context.WithCancel(ctx)

	return &session{
		agentConfig:                     config,
		deregisterInstanceEventStream:   deregisterInstanceEventStream,
		containerInstanceARN:            containerInstanceARN,
		credentialsProvider:             credentialsProvider,
		ecsClient:                       ecsClient,
		dockerClient:                    dockerClient,
		state:                           taskEngineState,
		dataClient:                      dataClient,
		taskEngine:                      taskEngine,
		credentialsManager:              credentialsManager,
		taskHandler:                     taskHandler,
		ctx:                             derivedContext,
		cancel:                          cancel,
		backoff:                         backoff,
		latestSeqNumTaskManifest:        latestSeqNumTaskManifest,
		doctor:                          doctor,
		clientFactory:                   clientFactory,
		sendCredentials:                 true,
		_heartbeatTimeout:               heartbeatTimeout,
		_heartbeatJitter:                heartbeatJitter,
		connectionTime:                  connectionTime,
		connectionJitter:                connectionJitter,
		_inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
	}
}

// Start starts the session. It'll forever keep trying to connect to ACS unless
// the context is cancelled.
//
// Returns nil always TODO: consider removing error return value completely
func (acsSession *session) Start() error {
	// Loop continuously until context is closed/cancelled
	for {
		seelog.Debugf("Attempting connect to ACS")
		// Start a session with ACS
		acsError := acsSession.startSessionOnce()

		// If the session is over check for shutdown first
		if err := acsSession.ctx.Err(); err != nil {
			return nil
		}

		// If ACS closed the connection, reconnect immediately
		if shouldReconnectWithoutBackoff(acsError) {
			seelog.Infof("ACS Websocket connection closed for a valid reason: %v", acsError)
			acsSession.backoff.Reset()
			continue
		}

		// Session with ACS was stopped with some error, start processing the error
		isInactiveInstance := isInactiveInstanceError(acsError)
		if isInactiveInstance {
			// If the instance was deregistered, send an event to the event stream
			// for the same
			seelog.Debug("Container instance is deregistered, notifying listeners")
			err := acsSession.deregisterInstanceEventStream.WriteToEventStream(struct{}{})
			if err != nil {
				seelog.Debugf("Failed to write to deregister container instance event stream, err: %v", err)
			}
		}

		// Disconnected unexpectedly from ACS, compute backoff duration to
		// reconnect
		reconnectDelay := acsSession.computeReconnectDelay(isInactiveInstance)
		seelog.Infof("Reconnecting to ACS in: %s", reconnectDelay.String())
		waitComplete := acsSession.waitForDuration(reconnectDelay)
		if !waitComplete {
			// Wait was interrupted. We expect the session to close as canceling
			// the session context is the only way to end up here. Print a message
			// to indicate the same
			seelog.Info("Interrupted waiting for reconnect delay to elapse; Expect session to close")
			return nil
		}

		// If the context was not cancelled and we've waited for the
		// wait duration without any errors, reconnect to ACS
		seelog.Info("Done waiting; reconnecting to ACS")
	}
}

// startSessionOnce creates a session with ACS and handles requests using the passed
// in arguments
func (acsSession *session) startSessionOnce() error {
	minAgentCfg := &wsclient.WSClientMinAgentConfig{
		AcceptInsecureCert: acsSession.agentConfig.AcceptInsecureCert,
		AWSRegion:          acsSession.agentConfig.AWSRegion,
	}

	acsEndpoint, err := acsSession.ecsClient.DiscoverPollEndpoint(acsSession.containerInstanceARN)
	if err != nil {
		seelog.Errorf("acs: unable to discover poll endpoint, err: %v", err)
		return err
	}

	url := acsSession.acsURL(acsEndpoint)
	client := acsSession.clientFactory.New(
		url,
		acsSession.credentialsProvider,
		wsRWTimeout,
		minAgentCfg)
	defer client.Close()

	return acsSession.startACSSession(client)
}

// startACSSession starts a session with ACS. It adds request handlers for various
// kinds of messages expected from ACS. It returns on server disconnection or when
// the context is cancelled
func (acsSession *session) startACSSession(client wsclient.ClientServer) error {
	cfg := acsSession.agentConfig

	refreshCredsHandler := newRefreshCredentialsHandler(acsSession.ctx, cfg.Cluster, acsSession.containerInstanceARN,
		client, acsSession.credentialsManager, acsSession.taskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	defer refreshCredsHandler.stop()

	client.AddRequestHandler(refreshCredsHandler.handlerFunc())

	eniHandler := &eniHandler{
		state:      acsSession.state,
		dataClient: acsSession.dataClient,
	}

	// Add handler to ack task ENI attach message
	eniAttachHandler := newAttachTaskENIHandler(
		acsSession.ctx,
		cfg.Cluster,
		acsSession.containerInstanceARN,
		client,
		eniHandler,
	)
	eniAttachHandler.start()
	defer eniAttachHandler.stop()

	client.AddRequestHandler(eniAttachHandler.handlerFunc())

	// Add handler to ack instance ENI attach message
	instanceENIAttachHandler := newAttachInstanceENIHandler(
		acsSession.ctx,
		cfg.Cluster,
		acsSession.containerInstanceARN,
		client,
		eniHandler,
	)
	instanceENIAttachHandler.start()
	defer instanceENIAttachHandler.stop()

	client.AddRequestHandler(instanceENIAttachHandler.handlerFunc())

	manifestMessageIDAccessor := &manifestMessageIDAccessor{}

	// Add TaskManifestHandler
	taskManifestHandler := newTaskManifestHandler(acsSession.ctx, cfg.Cluster, acsSession.containerInstanceARN,
		client, acsSession.dataClient, acsSession.taskEngine, acsSession.latestSeqNumTaskManifest,
		manifestMessageIDAccessor)

	defer taskManifestHandler.clearAcks()
	taskManifestHandler.start()
	defer taskManifestHandler.stop()

	client.AddRequestHandler(taskManifestHandler.handlerFuncTaskManifestMessage())
	client.AddRequestHandler(taskManifestHandler.handlerFuncTaskStopVerificationMessage())

	// Add request handler for handling payload messages from ACS
	payloadHandler := newPayloadRequestHandler(
		acsSession.ctx,
		acsSession.taskEngine,
		acsSession.ecsClient,
		cfg.Cluster,
		acsSession.containerInstanceARN,
		client,
		acsSession.dataClient,
		refreshCredsHandler,
		acsSession.credentialsManager,
		acsSession.taskHandler, acsSession.latestSeqNumTaskManifest)
	// Clear the acks channel on return because acks of messageids don't have any value across sessions
	defer payloadHandler.clearAcks()
	payloadHandler.start()
	defer payloadHandler.stop()

	client.AddRequestHandler(payloadHandler.handlerFunc())

	client.AddRequestHandler(HeartbeatHandlerFunc(client, acsSession.doctor))

	updater.AddAgentUpdateHandlers(client, cfg, acsSession.state, acsSession.dataClient, acsSession.taskEngine)

	err := client.Connect()
	if err != nil {
		seelog.Errorf("Error connecting to ACS: %v", err)
		return err
	}

	seelog.Info("Connected to ACS endpoint")
	// Start a connection timer; agent will send pending acks and close its ACS websocket connection
	// after this timer expires
	connectionTimer := newConnectionTimer(client, acsSession.connectionTime, acsSession.connectionJitter,
		&refreshCredsHandler, &taskManifestHandler, &payloadHandler)
	defer connectionTimer.Stop()

	// Start a heartbeat timer for closing the connection
	heartbeatTimer := newHeartbeatTimer(client, acsSession.heartbeatTimeout(), acsSession.heartbeatJitter())
	// Any message from the server resets the heartbeat timer
	client.SetAnyRequestHandler(anyMessageHandler(heartbeatTimer, client))
	defer heartbeatTimer.Stop()

	// Connection to ACS was successful. Moving forward, rely on ACS to send credentials to Agent at its own cadence
	// and make sure Agent does not force ACS to send credentials for any subsequent reconnects to ACS.
	acsSession.sendCredentials = false

	backoffResetTimer := time.AfterFunc(
		retry.AddJitter(acsSession.heartbeatTimeout(), acsSession.heartbeatJitter()), func() {
			// If we do not have an error connecting and remain connected for at
			// least 1 or so minutes, reset the backoff. This prevents disconnect
			// errors that only happen infrequently from damaging the reconnect
			// delay as significantly.
			acsSession.backoff.Reset()
		})
	defer backoffResetTimer.Stop()

	return client.Serve(acsSession.ctx)
}

func (acsSession *session) computeReconnectDelay(isInactiveInstance bool) time.Duration {
	if isInactiveInstance {
		return acsSession._inactiveInstanceReconnectDelay
	}

	return acsSession.backoff.Duration()
}

// waitForDuration waits for the specified duration of time. If the wait is interrupted,
// it returns a false value. Else, it returns true, indicating completion of wait time.
func (acsSession *session) waitForDuration(delay time.Duration) bool {
	reconnectTimer := time.NewTimer(delay)
	select {
	case <-reconnectTimer.C:
		return true
	case <-acsSession.ctx.Done():
		reconnectTimer.Stop()
		return false
	}
}

func (acsSession *session) heartbeatTimeout() time.Duration {
	return acsSession._heartbeatTimeout
}

func (acsSession *session) heartbeatJitter() time.Duration {
	return acsSession._heartbeatJitter
}

// acsURL returns the websocket url for ACS given the endpoint
func (acsSession *session) acsURL(endpoint string) string {
	acsURL := endpoint
	if endpoint[len(endpoint)-1] != '/' {
		acsURL += "/"
	}
	acsURL += "ws"
	query := url.Values{}
	query.Set("clusterArn", acsSession.agentConfig.Cluster)
	query.Set("containerInstanceArn", acsSession.containerInstanceARN)
	query.Set("agentHash", version.GitHashString())
	query.Set("agentVersion", version.Version)
	query.Set("seqNum", "1")
	query.Set("protocolVersion", strconv.Itoa(acsProtocolVersion))
	if dockerVersion, err := acsSession.taskEngine.Version(); err == nil {
		query.Set("dockerVersion", "DockerVersion: "+dockerVersion)
	}
	query.Set(sendCredentialsURLParameterName, strconv.FormatBool(acsSession.sendCredentials))
	return acsURL + "?" + query.Encode()
}

// newHeartbeatTimer creates a new time object, with a callback to
// disconnect from ACS on inactivity
func newHeartbeatTimer(client wsclient.ClientServer, timeout time.Duration, jitter time.Duration) ttime.Timer {
	timer := time.AfterFunc(retry.AddJitter(timeout, jitter), func() {
		seelog.Warn("ACS Connection hasn't had any activity for too long; closing connection")
		if err := client.Close(); err != nil {
			seelog.Warnf("Error disconnecting: %v", err)
		}
		seelog.Info("Disconnected from ACS")
	})

	return timer
}

// newConnectionTimer creates a new timer, after which agent sends any pending acks to ACS and closes
// its websocket connection
func newConnectionTimer(
	client wsclient.ClientServer,
	connectionTime time.Duration,
	connectionJitter time.Duration,
	refreshCredsHandler *refreshCredentialsHandler,
	taskManifestHandler *taskManifestHandler,
	payloadHandler *payloadRequestHandler,
) ttime.Timer {
	expiresAt := retry.AddJitter(connectionTime, connectionJitter)
	timer := time.AfterFunc(expiresAt, func() {
		seelog.Debugf("Sending pending acks to ACS before closing the connection")

		wg := sync.WaitGroup{}
		wg.Add(numOfHandlersSendingAcks)

		// send pending creds refresh acks to ACS
		go func() {
			refreshCredsHandler.sendPendingAcks()
			wg.Done()
		}()

		// send pending task manifest acks and task stop verification acks to ACS
		go func() {
			taskManifestHandler.sendPendingTaskManifestMessageAck()
			taskManifestHandler.handlePendingTaskStopVerificationAck()
			wg.Done()
		}()

		// send pending payload acks to ACS
		go func() {
			payloadHandler.sendPendingAcks()
			wg.Done()
		}()

		// wait for acks from all the handlers above to be sent to ACS before closing the websocket connection.
		// the methods used to read pending acks are non-blocking, so it is safe to wait here.
		wg.Wait()

		seelog.Infof("Closing ACS websocket connection after %v minutes", expiresAt.Minutes())
		// WriteCloseMessage() writes a close message using websocket control messages
		// Ref: https://pkg.go.dev/github.com/gorilla/websocket#hdr-Control_Messages
		err := client.WriteCloseMessage()
		if err != nil {
			seelog.Warnf("Error writing close message: %v", err)
		}
	})
	return timer
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active and thus the heartbeat disconnect should not occur
func anyMessageHandler(timer ttime.Timer, client wsclient.ClientServer) func(interface{}) {
	return func(interface{}) {
		seelog.Debug("ACS activity occurred")
		// Reset read deadline as there's activity on the channel
		if err := client.SetReadDeadline(time.Now().Add(wsRWTimeout)); err != nil {
			seelog.Warnf("Unable to extend read deadline for ACS connection: %v", err)
		}

		// Reset heartbeat timer
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

func shouldReconnectWithoutBackoff(acsError error) bool {
	return acsError == nil || acsError == io.EOF
}

func isInactiveInstanceError(acsError error) bool {
	return acsError != nil && strings.HasPrefix(acsError.Error(), inactiveInstanceExceptionPrefix)
}
