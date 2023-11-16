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

// Package session deals with appropriately reacting to all ACS messages as well
// as maintaining the connection to ACS.
package session

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api"
	rolecredentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const (
	// heartbeatTimeout is the maximum time to wait between heartbeats
	// without disconnecting.
	heartbeatTimeout = 1 * time.Minute
	heartbeatJitter  = 1 * time.Minute

	// wsRWTimeout is the duration of read and write deadline for the
	// websocket connection.
	wsRWTimeout = 2*heartbeatTimeout + heartbeatJitter

	inactiveInstanceReconnectDelay = 1 * time.Hour

	connectionBackoffMin        = 250 * time.Millisecond
	connectionBackoffMax        = 2 * time.Minute
	connectionBackoffJitter     = 0.2
	connectionBackoffMultiplier = 1.5

	inactiveInstanceExceptionPrefix = "InactiveInstanceException"

	// ACS protocol version spec:
	// 1: default protocol version
	// 2: ACS will proactively close the connection when heartbeat ACKs are missing
	acsProtocolVersion = 2
)

// Session defines an interface for Agent's long-lived connection with ACS.
// The Session.Start() method can be used to start processing messages from ACS.
type Session interface {
	Start(context.Context) error
}

// session encapsulates all arguments needed to connect to ACS and to handle messages received by ACS.
type session struct {
	containerInstanceARN           string
	cluster                        string
	credentialsProvider            *credentials.Credentials
	discoverEndpointClient         api.ECSDiscoverEndpointSDK
	inactiveInstanceCB             func()
	agentVersion                   string
	agentHash                      string
	dockerVersion                  string
	payloadMessageHandler          PayloadMessageHandler
	credentialsManager             rolecredentials.Manager
	credentialsMetadataSetter      CredentialsMetadataSetter
	doctor                         *doctor.Doctor
	eniHandler                     ENIHandler
	manifestMessageIDAccessor      ManifestMessageIDAccessor
	taskComparer                   TaskComparer
	sequenceNumberAccessor         SequenceNumberAccessor
	taskStopper                    TaskStopper
	resourceHandler                ResourceHandler
	backoff                        retry.Backoff
	sendCredentials                bool
	clientFactory                  wsclient.ClientFactory
	metricsFactory                 metrics.EntryFactory
	minAgentConfig                 *wsclient.WSClientMinAgentConfig
	addUpdateRequestHandlers       func(wsclient.ClientServer)
	heartbeatTimeout               time.Duration
	heartbeatJitter                time.Duration
	disconnectTimeout              time.Duration
	disconnectJitter               time.Duration
	inactiveInstanceReconnectDelay time.Duration
}

// NewSession creates a new Session.
func NewSession(containerInstanceARN string,
	cluster string,
	discoverEndpointClient api.ECSDiscoverEndpointSDK,
	credentialsProvider *credentials.Credentials,
	inactiveInstanceCB func(),
	clientFactory wsclient.ClientFactory,
	metricsFactory metrics.EntryFactory,
	agentVersion string,
	agentHash string,
	dockerVersion string,
	minAgentConfig *wsclient.WSClientMinAgentConfig,
	payloadMessageHandler PayloadMessageHandler,
	credentialsManager rolecredentials.Manager,
	credentialsMetadataSetter CredentialsMetadataSetter,
	doctor *doctor.Doctor,
	eniHandler ENIHandler,
	manifestMessageIDAccessor ManifestMessageIDAccessor,
	taskComparer TaskComparer,
	sequenceNumberAccessor SequenceNumberAccessor,
	taskStopper TaskStopper,
	resourceHandler ResourceHandler,
	addUpdateRequestHandlers func(wsclient.ClientServer),
) Session {
	backoff := retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
		connectionBackoffJitter, connectionBackoffMultiplier)
	return &session{
		containerInstanceARN:           containerInstanceARN,
		cluster:                        cluster,
		discoverEndpointClient:         discoverEndpointClient,
		credentialsProvider:            credentialsProvider,
		inactiveInstanceCB:             inactiveInstanceCB,
		clientFactory:                  clientFactory,
		metricsFactory:                 metricsFactory,
		agentVersion:                   agentVersion,
		agentHash:                      agentHash,
		dockerVersion:                  dockerVersion,
		minAgentConfig:                 minAgentConfig,
		payloadMessageHandler:          payloadMessageHandler,
		credentialsManager:             credentialsManager,
		credentialsMetadataSetter:      credentialsMetadataSetter,
		doctor:                         doctor,
		eniHandler:                     eniHandler,
		manifestMessageIDAccessor:      manifestMessageIDAccessor,
		taskComparer:                   taskComparer,
		sequenceNumberAccessor:         sequenceNumberAccessor,
		taskStopper:                    taskStopper,
		resourceHandler:                resourceHandler,
		addUpdateRequestHandlers:       addUpdateRequestHandlers,
		backoff:                        backoff,
		sendCredentials:                true,
		heartbeatTimeout:               heartbeatTimeout,
		heartbeatJitter:                heartbeatJitter,
		disconnectTimeout:              wsclient.DisconnectTimeout,
		disconnectJitter:               wsclient.DisconnectJitterMax,
		inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
	}
}

// Start starts the session. It'll forever keep trying to connect to ACS unless
// the context is closed.
//
// If the context is closed, Start() would return with the error code returned
// by the context.
func (s *session) Start(ctx context.Context) error {
	// connectToACS channel is used to indicate the intent to connect to ACS
	// It's processed by the select loop to connect to ACS.
	connectToACS := make(chan struct{})

	// The below is required to trigger the first connection to ACS.
	sendEmptyMessageOnChannel(connectToACS)

	// Loop continuously until context is closed/canceled.
	for {
		select {
		case <-connectToACS:
			logger.Debug("Received connect to ACS message. Attempting connect to ACS")

			// Start a session with ACS.
			acsError := s.startSessionOnce(ctx)

			// Session with ACS was stopped with some error, start processing the error.
			reconnectDelay, ok := s.reconnectDelay(acsError)

			if ok {
				logger.Info("Waiting before reconnecting to ACS", logger.Fields{
					"reconnectDelay": reconnectDelay.String(),
				})
				waitComplete := waitForDuration(ctx, reconnectDelay)
				if waitComplete {
					// If the context was not canceled and we've waited for the
					// wait duration without any errors, send the message to the channel
					// to reconnect to ACS.
					logger.Info("Done waiting; reconnecting to ACS")
					sendEmptyMessageOnChannel(connectToACS)
				} else {
					// Wait was interrupted. We expect the session to close as canceling
					// the session context is the only way to end up here. Print a message
					// to indicate the same.
					logger.Info("Interrupted waiting for reconnect delay to elapse; Expect session to close")
				}
			} else {
				// No need to delay reconnect - reconnect immediately.
				logger.Info("Reconnecting to ACS immediately without waiting")
				sendEmptyMessageOnChannel(connectToACS)
			}
		case <-ctx.Done():
			logger.Info("ACS session ended (context closed)", logger.Fields{
				field.Reason: ctx.Err(),
			})
			return ctx.Err()
		}
	}
}

// startSessionOnce creates a session with ACS and handles requests using the passed
// in arguments.
func (s *session) startSessionOnce(ctx context.Context) error {
	acsEndpoint, err := s.discoverEndpointClient.DiscoverPollEndpoint(s.containerInstanceARN)
	if err != nil {
		logger.Error("ACS: Unable to discover poll endpoint", logger.Fields{
			field.Error: err,
		})
		return err
	}

	client := s.clientFactory.New(
		s.acsURL(acsEndpoint),
		s.credentialsProvider,
		wsRWTimeout,
		s.minAgentConfig,
		s.metricsFactory)
	defer client.Close()

	// Invoke Connect method as soon as we create client. This will ensure all the
	// request handlers to be associated with this client have a valid connection.
	disconnectTimer, err := client.Connect(metrics.ACSDisconnectTimeoutMetricName, s.disconnectTimeout,
		s.disconnectJitter)
	if err != nil {
		logger.Error("Failed to connect to ACS", logger.Fields{
			field.Error: err,
		})
		return err
	}
	defer disconnectTimer.Stop()

	// Connection to ACS was successful. Moving forward, rely on ACS to send credentials to Agent at its own cadence
	// and make sure Agent does not force ACS to send credentials for any subsequent reconnects to ACS.
	logger.Info("Connected to ACS endpoint")
	s.sendCredentials = false

	return s.startACSSession(ctx, client)
}

// startACSSession starts a session with ACS. It adds request handlers for various
// kinds of messages expected from ACS. It returns on server disconnection or when
// the context is canceled.
func (s *session) startACSSession(ctx context.Context, client wsclient.ClientServer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responseSender := func(response interface{}) error {
		return client.MakeRequest(response)
	}
	responders := []wsclient.RequestResponder{
		NewPayloadResponder(s.payloadMessageHandler, responseSender),
		NewRefreshCredentialsResponder(s.credentialsManager, s.credentialsMetadataSetter, s.metricsFactory,
			responseSender),
		NewAttachTaskENIResponder(s.eniHandler, responseSender),
		NewAttachInstanceENIResponder(s.eniHandler, responseSender),
		NewHeartbeatResponder(s.doctor, responseSender),
		NewTaskManifestResponder(s.taskComparer, s.sequenceNumberAccessor, s.manifestMessageIDAccessor,
			s.metricsFactory, responseSender),
		NewTaskStopVerificationACKResponder(s.taskStopper, s.manifestMessageIDAccessor, s.metricsFactory),
	}
	for _, r := range responders {
		client.AddRequestHandler(r.HandlerFunc())
	}

	if s.resourceHandler != nil {
		client.AddRequestHandler(NewAttachResourceResponder(s.resourceHandler, s.metricsFactory,
			responseSender).HandlerFunc())
	}

	if s.dockerVersion != "containerd" && s.addUpdateRequestHandlers != nil {
		s.addUpdateRequestHandlers(client)
	}

	// Start a heartbeat timer for closing the connection.
	heartbeatTimer := newHeartbeatTimer(client, s.heartbeatTimeout, s.heartbeatJitter)
	// Any message from the server resets the heartbeat timer.
	client.SetAnyRequestHandler(anyMessageHandler(heartbeatTimer, client))
	defer heartbeatTimer.Stop()

	backoffResetTimer := time.AfterFunc(
		retry.AddJitter(s.heartbeatTimeout, s.heartbeatJitter), func() {
			// If we do not have an error connecting and remain connected for at
			// least 1 or so minutes, reset the backoff. This prevents disconnect
			// errors that only happen infrequently from damaging the reconnect
			// delay as significantly.
			s.backoff.Reset()
		})
	defer backoffResetTimer.Stop()

	return client.Serve(ctx)
}

func (s *session) reconnectDelay(acsError error) (time.Duration, bool) {
	if isInactiveInstanceError(acsError) {
		logger.Info("Container instance is deregistered")
		s.inactiveInstanceCB()
		return s.inactiveInstanceReconnectDelay, true
	}
	if shouldReconnectWithoutBackoff(acsError) {
		// ACS has closed the connection for valid reasons. Example: periodic disconnect.
		// No need to wait/backoff to reconnect.
		logger.Info("ACS WebSocket connection closed for a valid reason")
		s.backoff.Reset()
		return 0, false

	}
	// Disconnected unexpectedly from ACS, compute backoff duration to reconnect.
	return s.backoff.Duration(), true
}

// acsURL returns the websocket url for ACS given the endpoint.
func (s *session) acsURL(endpoint string) string {
	wsURL := endpoint
	if endpoint[len(endpoint)-1] != '/' {
		wsURL += "/"
	}
	wsURL += "ws"
	query := url.Values{}
	query.Set("clusterArn", s.cluster)
	query.Set("containerInstanceArn", s.containerInstanceARN)
	query.Set("agentHash", s.agentHash)
	query.Set("agentVersion", s.agentVersion)
	query.Set("seqNum", "1")
	query.Set("protocolVersion", strconv.Itoa(acsProtocolVersion))
	if s.dockerVersion != "" {
		query.Set("dockerVersion", formatDockerVersion(s.dockerVersion))
	}
	// Below indicates if ACS should send credentials for all tasks upon establishing the connection.
	query.Set("sendCredentials", strconv.FormatBool(s.sendCredentials))
	return wsURL + "?" + query.Encode()
}

// responseToACSSender returns a wsclient.RespondFunc that a responder can invoke in response to receiving and
// processing specific websocket request messages from ACS. The returned wsclient.RespondFunc:
//  1. logs the response to be sent, as well as the name of the invoking responder
//  2. sends the response request to ACS
func responseToACSSender(responderName string, responseSender wsclient.RespondFunc) wsclient.RespondFunc {
	return func(response interface{}) error {
		logger.Debug("Sending response to ACS", logger.Fields{
			"Name":     responderName,
			"Response": response,
		})
		return responseSender(response)
	}
}

// newHeartbeatTimer creates a new time object, with a callback to
// disconnect from ACS on inactivity (i.e., after timeout + jitter).
func newHeartbeatTimer(client wsclient.ClientServer, timeout time.Duration, jitter time.Duration) ttime.Timer {
	timer := time.AfterFunc(retry.AddJitter(timeout, jitter), func() {
		logger.Warn("ACS Connection hasn't had any activity for too long; closing connection")
		if err := client.Close(); err != nil {
			logger.Warn("Error disconnecting from ACS", logger.Fields{
				field.Error: err,
			})
		}
		logger.Info("Disconnected from ACS")
	})

	return timer
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active and thus the heartbeat disconnect should not occur.
func anyMessageHandler(timer ttime.Timer, client wsclient.ClientServer) func(interface{}) {
	return func(interface{}) {
		logger.Debug("ACS activity occurred")
		// Reset read deadline as there's activity on the channel.
		if err := client.SetReadDeadline(time.Now().Add(wsRWTimeout)); err != nil {
			logger.Warn("Unable to extend read deadline for ACS connection", logger.Fields{
				field.Error: err,
			})
		}

		// Reset heartbeat timer.
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// waitForDuration waits for the specified duration of time. It returns true if the wait time has completed.
// Else, it returns false.
func waitForDuration(ctx context.Context, duration time.Duration) bool {
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(duration))
	defer cancel()
	<-ctx.Done()
	err := ctx.Err()
	return err == context.DeadlineExceeded
}

// sendEmptyMessageOnChannel sends an empty message using a goroutine on the
// specified channel.
func sendEmptyMessageOnChannel(channel chan<- struct{}) {
	go func() {
		channel <- struct{}{}
	}()
}

func shouldReconnectWithoutBackoff(acsError error) bool {
	return acsError == nil || acsError == io.EOF
}

func isInactiveInstanceError(acsError error) bool {
	return acsError != nil && strings.Contains(acsError.Error(), inactiveInstanceExceptionPrefix)
}

func formatDockerVersion(dockerVersionValue string) string {
	if dockerVersionValue != "containerd" {
		return "DockerVersion: " + dockerVersionValue
	}
	return dockerVersionValue
}
