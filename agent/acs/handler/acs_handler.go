// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	acsclient "github.com/aws/amazon-ecs-agent/agent/acs/client"
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	rolecredentials "github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	// heartbeatTimeout is the maximum time to wait between heartbeats
	// without disconnecting
	heartbeatTimeout = 5 * time.Minute
	heartbeatJitter  = 3 * time.Minute

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
)

type Session struct {
	containerInstanceARN          string
	credentialsProvider           *credentials.Credentials
	agentConfig                   *config.Config
	deregisterInstanceEventStream *eventstream.EventStream
	taskEngine                    engine.TaskEngine
	ecsClient                     api.ECSClient
	StateManager                  statemanager.StateManager
	acceptInsecureCert            bool
	credentialsManager            rolecredentials.Manager
	ctx                           context.Context
	cancel                        context.CancelFunc
	backoff                       *utils.SimpleBackoff
	resources                     sessionResources
	reconnectStoppedNotification  sync.Once
	_time                         ttime.Time
	_heartbeatTimeout             time.Duration
	_heartbeatJitter              time.Duration
	_timeOnce                     sync.Once
}

// sessionResources defines the resource creator interface for starting
// a session with ACS. This interface is intended to define methods
// that create resources used to establish the connection to ACS
// It is confined to just the createACSClient() method for now. It can be
// extended to include the acsWsURL() and newDisconnectionTimer() methods
// when needed
// The goal is to make it easier to test and inject dependencies
type sessionResources interface {
	// createACSClient creates a new websocket client
	createACSClient(url string) wsclient.ClientServer
	sessionState
}

// acsSessionResources implements resource creator and session state interfaces
// to create resources needed to connect to ACS and to record session state
// for the same
type acsSessionResources struct {
	region              string
	credentialsProvider *credentials.Credentials
	acceptInsecureCert  bool
	// sendCredentials is used to set the 'sendCredentials' URL parameter
	// used to connect to ACS
	// It is set to 'true' for the very first successful connection on
	// agent start. It is set to false for all successive connections
	sendCredentials bool
}

// sessionState defines state recorder interface for the
// session established with ACS. It can be used to record and
// retrieve data shared across multiple connections to ACS
type sessionState interface {
	// connectedToACS callback indicates that the client has
	// connected to ACS
	connectedToACS()
	// getSendCredentialsURLParameter retrieves the value for
	// the 'sendCredentials' URL parameter
	getSendCredentialsURLParameter() string
}

func NewSession(ctx context.Context,
	acceptInsecureCert bool,
	config *config.Config,
	deregisterInstanceEventStream *eventstream.EventStream,
	containerInstanceArn string,
	credentialsProvider *credentials.Credentials,
	ecsClient api.ECSClient,
	stateManager statemanager.StateManager,
	taskEngine engine.TaskEngine,
	credentialsManager rolecredentials.Manager) *Session {
	resources := newSessionResources(config.AWSRegion, credentialsProvider, acceptInsecureCert)
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax,
		connectionBackoffJitter, connectionBackoffMultiplier)
	derivedContext, cancel := context.WithCancel(ctx)

	return &Session{
		acceptInsecureCert:            acceptInsecureCert,
		agentConfig:                   config,
		deregisterInstanceEventStream: deregisterInstanceEventStream,
		containerInstanceARN:          containerInstanceArn,
		credentialsProvider:           credentialsProvider,
		ecsClient:                     ecsClient,
		StateManager:                  stateManager,
		taskEngine:                    taskEngine,
		credentialsManager:            credentialsManager,
		ctx:                           derivedContext,
		cancel:                        cancel,
		backoff:                       backoff,
		resources:                     resources,
	}
}

// Start starts the session. It'll forever keep trying to connect to ACS unless:
// 1. The context is cancelled
// 2. Instance is deregistered
func (s *Session) Start() error {
	connectToACS := true
	for {
		if connectToACS {
			acsError := s.startSessionOnce()
			connectToACS = s.handleACSError(acsError)
		} else {
			s.reconnectStoppedNotification.Do(func() {
				seelog.Info("Stopping reconnect attempts to ACS")
			})
		}
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

	}
}

// startSessionOnce creates a session with ACS and handles requests using the passed
// in arguments
func (s *Session) startSessionOnce() error {
	acsEndpoint, err := s.ecsClient.DiscoverPollEndpoint(s.containerInstanceARN)
	if err != nil {
		seelog.Errorf("Unable to discover poll endpoint, err: %v", err)
		return err
	}

	url := acsWsURL(acsEndpoint, s.agentConfig.Cluster, s.containerInstanceARN, s.taskEngine, s.resources)
	client := s.resources.createACSClient(url)
	defer client.Close()

	// Start inactivity timer for closing the connection
	timer := newDisconnectionTimer(client, s.time(), s.heartbeatTimeout(), s.heartbeatJitter())
	defer timer.Stop()

	return s.startACSSession(client, timer)
}

// startACSSession starts a session with ACS. It adds request handlers for various
// kinds of messages expected from ACS. It returns on server disconnection or when
// the context is cancelled
func (s *Session) startACSSession(client wsclient.ClientServer, timer ttime.Timer) error {
	// Any message from the server resets the disconnect timeout
	client.SetAnyRequestHandler(anyMessageHandler(timer))
	cfg := s.agentConfig

	refreshCredsHandler := newRefreshCredentialsHandler(s.ctx, cfg.Cluster, s.containerInstanceARN,
		client, s.credentialsManager, s.taskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	defer refreshCredsHandler.stop()

	client.AddRequestHandler(refreshCredsHandler.handlerFunc())

	// Add request handler for handling payload messages from ACS
	payloadHandler := newPayloadRequestHandler(s.ctx, s.taskEngine, s.ecsClient, cfg.Cluster,
		s.containerInstanceARN, client, s.StateManager, refreshCredsHandler, s.credentialsManager)
	// Clear the acks channel on return because acks of messageids don't have any value across sessions
	defer payloadHandler.clearAcks()
	payloadHandler.start()
	defer payloadHandler.stop()

	client.AddRequestHandler(payloadHandler.handlerFunc())

	// Ignore heartbeat messages; anyMessageHandler gets 'em
	client.AddRequestHandler(func(*ecsacs.HeartbeatMessage) {})

	updater.AddAgentUpdateHandlers(client, cfg, s.StateManager, s.taskEngine)

	err := client.Connect()
	if err != nil {
		seelog.Errorf("Error connecting to ACS: %v", err)
		return err
	}
	s.resources.connectedToACS()

	backoffResetTimer := s.time().AfterFunc(
		utils.AddJitter(s.heartbeatTimeout(), s.heartbeatJitter()), func() {
			// If we do not have an error connecting and remain connected for at
			// least 5 or so minutes, reset the backoff. This prevents disconnect
			// errors that only happen infrequently from damaging the
			// reconnectability as significantly.
			s.backoff.Reset()
		})
	defer backoffResetTimer.Stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- client.Serve()
	}()

	for {
		select {
		case <-s.ctx.Done():
			// Stop receiving and sending messages from and to ACS when
			// the context received from the main function is canceled
			return s.ctx.Err()
		case err := <-serveErr:
			// Stop receiving and sending messages from and to ACS when
			// client.Serve returns an error. This can happen when the
			// the connection is closed by ACS or the agent
			return err
		}
	}
}

// handleACSError handles an error from ACS. It returns true if the session can reconnect
// to ACS. Else, it returns false. It also applies the appropriate backoff policy for
// errors which can result in the handler reconnecting to ACS. For an "InactiveInstance"
// error, it emits an event to the deregister instance event stream.
func (s *Session) handleACSError(acsError error) bool {
	if acsError == nil || acsError == io.EOF {
		s.backoff.Reset()
	} else if strings.HasPrefix(acsError.Error(), "InactiveInstanceException:") {
		seelog.Debug("Container instance is deregistered, notifying listeners")
		err := s.deregisterInstanceEventStream.WriteToEventStream(struct{}{})
		if err != nil {
			seelog.Debugf("Failed to write to deregister container instance event stream, err: %v", err)
		}
		return false
	} else {
		seelog.Infof("Error from acs; backing off, err: %v", acsError)
		s.time().Sleep(s.backoff.Duration())
	}

	return true
}

func (s *Session) time() ttime.Time {
	s.initTime()
	return s._time
}

func (s *Session) heartbeatTimeout() time.Duration {
	s.initTime()
	return s._heartbeatTimeout
}

func (s *Session) heartbeatJitter() time.Duration {
	s.initTime()
	return s._heartbeatJitter
}

func (s *Session) initTime() {
	s._timeOnce.Do(func() {
		if s._time == nil {
			s._time = &ttime.DefaultTime{}
		}
		if s._heartbeatTimeout == 0 {
			s._heartbeatTimeout = heartbeatTimeout
		}
		if s._heartbeatJitter == 0 {
			s._heartbeatJitter = heartbeatJitter
		}
	})
}

// createACSClient creates the ACS Client using the specified URL
func (acsResources *acsSessionResources) createACSClient(url string) wsclient.ClientServer {
	return acsclient.New(
		url, acsResources.region, acsResources.credentialsProvider, acsResources.acceptInsecureCert)
}

// connectedToACS records a successful connection to ACS
// It sets sendCredentials to false on such an event
func (acsResources *acsSessionResources) connectedToACS() {
	acsResources.sendCredentials = false
}

// getSendCredentialsURLParameter gets the value to be set for the
// 'sendCredentials' URL parameter
func (acsResources *acsSessionResources) getSendCredentialsURLParameter() string {
	return strconv.FormatBool(acsResources.sendCredentials)
}

func newSessionResources(region string, credentialsProvider *credentials.Credentials, acceptInsecureCert bool) sessionResources {
	return &acsSessionResources{
		region:              region,
		credentialsProvider: credentialsProvider,
		acceptInsecureCert:  acceptInsecureCert,
		sendCredentials:     true,
	}
}

// acsWsURL returns the websocket url for ACS given the endpoint
func acsWsURL(endpoint, cluster, containerInstanceArn string, taskEngine engine.TaskEngine, acsSessionState sessionState) string {
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
	query.Set("seqNum", "1")
	if dockerVersion, err := taskEngine.Version(); err == nil {
		query.Set("dockerVersion", dockerVersion)
	}
	query.Set(sendCredentialsURLParameterName, acsSessionState.getSendCredentialsURLParameter())
	return acsUrl + "?" + query.Encode()
}

// newDisconnectionTimer creates a new time object, with a callback to
// disconnect from ACS on inactivity
func newDisconnectionTimer(client wsclient.ClientServer, _time ttime.Time, timeout time.Duration, jitter time.Duration) ttime.Timer {
	timer := _time.AfterFunc(utils.AddJitter(timeout, jitter), func() {
		seelog.Warn("ACS Connection hasn't had any activity for too long; closing connection")
		closeErr := client.Close()
		if closeErr != nil {
			seelog.Warnf("Error disconnecting: %v", closeErr)
		}
	})

	return timer
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active and thus the heartbeat disconnect should not occur
func anyMessageHandler(timer ttime.Timer) func(interface{}) {
	return func(interface{}) {
		seelog.Debug("ACS activity occured")
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}
