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

// StartSessionArguments is a struct representing all the things this handler
// needs... This is really a hack to get by-name instead of positional
// arguments since there are too many for positional to be wieldy
type StartSessionArguments struct {
	ContainerInstanceArn          string
	CredentialProvider            *credentials.Credentials
	Config                        *config.Config
	DeregisterInstanceEventStream *eventstream.EventStream
	TaskEngine                    engine.TaskEngine
	ECSClient                     api.ECSClient
	StateManager                  statemanager.StateManager
	AcceptInvalidCert             bool
	CredentialsManager            rolecredentials.Manager
	_time                         ttime.Time
	_heartbeatTimeout             time.Duration
	_heartbeatJitter              time.Duration
	_timeOnce                     sync.Once
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

func (a *StartSessionArguments) time() ttime.Time {
	a.initTime()
	return a._time
}

func (a *StartSessionArguments) heartbeatTimeout() time.Duration {
	a.initTime()
	return a._heartbeatTimeout
}

func (a *StartSessionArguments) heartbeatJitter() time.Duration {
	a.initTime()
	return a._heartbeatJitter
}

func (a *StartSessionArguments) initTime() {
	a._timeOnce.Do(func() {
		if a._time == nil {
			a._time = &ttime.DefaultTime{}
		}
		if a._heartbeatTimeout == 0 {
			a._heartbeatTimeout = heartbeatTimeout
		}
		if a._heartbeatJitter == 0 {
			a._heartbeatJitter = heartbeatJitter
		}
	})
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
	startSessionArguments StartSessionArguments
	// sendCredentials is used to set the 'sendCredentials' URL parameter
	// used to connect to ACS
	// It is set to 'true' for the very first successful connection on
	// agent start. It is set to false for all successive connections
	sendCredentials bool
}

// StartSession creates a session with ACS and handles requests from ACS.
// It creates resources required to invoke the package scoped 'startSession()'
// method and invokes the same to repeatedly connect to ACS when disconnected
func StartSession(ctx context.Context, args StartSessionArguments) error {
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	session := newSessionResources(args)
	return startSession(ctx, args, backoff, session)
}

func newSessionResources(args StartSessionArguments) sessionResources {
	return &acsSessionResources{
		startSessionArguments: args,
		sendCredentials:       true,
	}
}

// startSession creates a session with ACS and handles requests from ACS
// It also tries to repeatedly connect to ACS when disconnected
func startSession(ctx context.Context, args StartSessionArguments, backoff *utils.SimpleBackoff, acsResources sessionResources) error {
	for {
		acsError := startSessionOnce(ctx, args, backoff, acsResources)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if acsError == nil || acsError == io.EOF {
			backoff.Reset()
		} else if strings.HasPrefix(acsError.Error(), "InactiveInstanceException:") {
			seelog.Debug("Container instance is deregistered, notifying listeners")
			err := args.DeregisterInstanceEventStream.WriteToEventStream(struct{}{})
			if err != nil {
				seelog.Debugf("Failed to write to deregister container instance event stream, err: %v", err)
			}
		} else {
			seelog.Infof("Error from acs; backing off, err: %v", acsError)
			args.time().Sleep(backoff.Duration())
		}
	}
}

// startSessionOnce creates a session with ACS and handles requests using the passed
// in arguments
func startSessionOnce(ctx context.Context, args StartSessionArguments, backoff *utils.SimpleBackoff, acsResources sessionResources) error {
	acsEndpoint, err := args.ECSClient.DiscoverPollEndpoint(args.ContainerInstanceArn)
	if err != nil {
		seelog.Errorf("Unable to discover poll endpoint, err: %v", err)
		return err
	}

	cfg := args.Config
	url := acsWsURL(acsEndpoint, cfg.Cluster, args.ContainerInstanceArn, args.TaskEngine, acsResources)
	client := acsResources.createACSClient(url)
	defer client.Close()

	// Start inactivity timer for closing the connection
	timer := newDisconnectionTimer(client, args.time(), args.heartbeatTimeout(), args.heartbeatJitter())
	defer timer.Stop()

	return startACSSession(ctx, client, timer, args, backoff, acsResources)
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

// createACSClient creates the ACS Client using the specified URL
func (acsResources *acsSessionResources) createACSClient(url string) wsclient.ClientServer {
	args := acsResources.startSessionArguments
	cfg := args.Config
	return acsclient.New(url, cfg.AWSRegion, args.CredentialProvider, args.AcceptInvalidCert)
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

// startACSSession starts a session with ACS. It adds request handlers for various
// kinds of messages expected from ACS. It returns on server disconnection or when
// the context is cancelled
func startACSSession(ctx context.Context, client wsclient.ClientServer, timer ttime.Timer, args StartSessionArguments, backoff *utils.SimpleBackoff, acsSessionState sessionState) error {
	// Any message from the server resets the disconnect timeout
	client.SetAnyRequestHandler(anyMessageHandler(timer))
	cfg := args.Config

	refreshCredsHandler := newRefreshCredentialsHandler(ctx, cfg.Cluster, args.ContainerInstanceArn, client, args.CredentialsManager, args.TaskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	defer refreshCredsHandler.stop()

	client.AddRequestHandler(refreshCredsHandler.handlerFunc())

	// Add request handler for handling payload messages from ACS
	payloadHandler := newPayloadRequestHandler(ctx, args.TaskEngine, args.ECSClient, cfg.Cluster, args.ContainerInstanceArn, client, args.StateManager, refreshCredsHandler, args.CredentialsManager)
	// Clear the acks channel on return because acks of messageids don't have any value across sessions
	defer payloadHandler.clearAcks()
	payloadHandler.start()
	defer payloadHandler.stop()

	client.AddRequestHandler(payloadHandler.handlerFunc())

	// Ignore heartbeat messages; anyMessageHandler gets 'em
	client.AddRequestHandler(func(*ecsacs.HeartbeatMessage) {})

	updater.AddAgentUpdateHandlers(client, cfg, args.StateManager, args.TaskEngine)

	err := client.Connect()
	if err != nil {
		seelog.Errorf("Error connecting to ACS: %v", err)
		return err
	}
	acsSessionState.connectedToACS()

	backoffResetTimer := args.time().AfterFunc(utils.AddJitter(args.heartbeatTimeout(), args.heartbeatJitter()), func() {
		// If we do not have an error connecting and remain connected for at
		// least 5 or so minutes, reset the backoff. This prevents disconnect
		// errors that only happen infrequently from damaging the
		// reconnectability as significantly.
		backoff.Reset()
	})
	defer backoffResetTimer.Stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- client.Serve()
	}()

	for {
		select {
		case <-ctx.Done():
			// Stop receiving and sending messages from and to ACS when
			// the context received from the main function is canceled
			return ctx.Err()
		case err := <-serveErr:
			// Stop receiving and sending messages from and to ACS when
			// client.Serve returns an error. This can happen when the
			// the connection is closed by ACS or the agent
			return err
		}
	}
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active and thus the heartbeat disconnect should not occur
func anyMessageHandler(timer ttime.Timer) func(interface{}) {
	return func(interface{}) {
		seelog.Debug("ACS activity occured")
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}
