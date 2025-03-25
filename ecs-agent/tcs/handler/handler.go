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

package tcshandler

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	tcsclient "github.com/aws/amazon-ecs-agent/ecs-agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	deregisterContainerInstanceHandler = "TCSDeregisterContainerInstanceHandler"
	ContainerRuntimeDocker             = "Docker"
	backoffMin                         = 1 * time.Second
	backoffMax                         = 1 * time.Minute
	jitterMultiple                     = 0.2
	multiple                           = 2
)

type TcsEcsClient interface {
	DiscoverTelemetryEndpoint(string) (string, error)
}

// TelemetrySession defines an interface for handler's long-lived connection with TCS.
type TelemetrySession interface {
	StartTelemetrySession(context.Context) error
	Start(context.Context) error
}

// telemetrySession is the base session params type which contains all the parameters required to start a tcs session
type telemetrySession struct {
	containerInstanceArn          string
	cluster                       string
	agentVersion                  string
	agentHash                     string
	containerRuntimeVersion       string
	disableMetrics                bool
	credentialsProvider           *credentials.Credentials
	cfg                           *wsclient.WSClientMinAgentConfig
	deregisterInstanceEventStream *eventstream.EventStream
	heartbeatTimeout              time.Duration
	heartbeatJitterMax            time.Duration
	disconnectTimeout             time.Duration
	disconnectJitterMax           time.Duration
	metricsFactory                metrics.EntryFactory
	metricsChannel                <-chan ecstcs.TelemetryMessage
	healthChannel                 <-chan ecstcs.HealthMessage
	doctor                        *doctor.Doctor
	ecsClient                     TcsEcsClient
}

func NewTelemetrySession(
	containerInstanceArn string,
	cluster string,
	agentVersion string,
	agentHash string,
	containerRuntimeVersion string,
	disableMetrics bool,
	credentialsProvider *credentials.Credentials,
	cfg *wsclient.WSClientMinAgentConfig,
	deregisterInstanceEventStream *eventstream.EventStream,
	heartbeatTimeout time.Duration,
	heartbeatJitterMax time.Duration,
	disconnectTimeout time.Duration,
	disconnectJitterMax time.Duration,
	metricsFactory metrics.EntryFactory,
	metricsChannel <-chan ecstcs.TelemetryMessage,
	healthChannel <-chan ecstcs.HealthMessage,
	doctor *doctor.Doctor,
	ecsClient TcsEcsClient,
) TelemetrySession {
	return &telemetrySession{
		containerInstanceArn:          containerInstanceArn,
		cluster:                       cluster,
		agentVersion:                  agentVersion,
		agentHash:                     agentHash,
		containerRuntimeVersion:       containerRuntimeVersion,
		disableMetrics:                disableMetrics,
		credentialsProvider:           credentialsProvider,
		cfg:                           cfg,
		deregisterInstanceEventStream: deregisterInstanceEventStream,
		metricsChannel:                metricsChannel,
		healthChannel:                 healthChannel,
		heartbeatTimeout:              heartbeatTimeout,
		heartbeatJitterMax:            heartbeatJitterMax,
		disconnectTimeout:             disconnectTimeout,
		disconnectJitterMax:           disconnectJitterMax,
		metricsFactory:                metricsFactory,
		doctor:                        doctor,
		ecsClient:                     ecsClient,
	}
}

// Start runs in for loop to start telemetry session with exponential backoff
func (session *telemetrySession) Start(ctx context.Context) error {
	backoff := retry.NewExponentialBackoff(backoffMin, backoffMax, jitterMultiple, multiple)
	for {
		select {
		case <-ctx.Done():
			logger.Info("ECS Telemetry service (TCS) session exited cleanly.")
			return nil
		default:
		}
		tcsError := session.StartTelemetrySession(ctx)
		switch tcsError {
		case context.Canceled, context.DeadlineExceeded:
			return tcsError
		case io.EOF, nil:
			logger.Info("TCS Websocket connection closed for a valid reason")
			backoff.Reset()
		default:
			seelog.Errorf("Error: lost websocket connection with ECS Telemetry service (TCS): %v", tcsError)
			time.Sleep(backoff.Duration())
		}
	}
}

// StartTelemetrySession creates a session with the backend and handles requests.
func (session *telemetrySession) StartTelemetrySession(ctx context.Context) error {
	wsRWTimeout := 2*session.heartbeatTimeout + session.heartbeatJitterMax

	var containerRuntime string
	if session.cfg.IsDocker {
		containerRuntime = ContainerRuntimeDocker
	}

	endpoint, err := session.getTelemetryEndpoint()
	if err != nil {
		return err
	}

	tcsEndpointUrl := formatURL(endpoint, session.cluster, session.containerInstanceArn, session.agentVersion,
		session.agentHash, containerRuntime, session.containerRuntimeVersion)
	client := tcsclient.New(tcsEndpointUrl, session.cfg, session.doctor, session.disableMetrics, tcsclient.DefaultContainerMetricsPublishInterval,
		session.credentialsProvider, wsRWTimeout, session.metricsChannel, session.healthChannel, session.metricsFactory)
	defer client.Close()

	if session.deregisterInstanceEventStream != nil {
		err := session.deregisterInstanceEventStream.Subscribe(deregisterContainerInstanceHandler, client.Disconnect)
		if err != nil {
			return err
		}
		defer session.deregisterInstanceEventStream.Unsubscribe(deregisterContainerInstanceHandler)
	}

	disconnectTimer, err := client.Connect(metrics.TCSDisconnectTimeoutMetricName,
		session.disconnectTimeout,
		session.disconnectJitterMax)
	if err != nil {
		logger.Error("Error connecting to TCS", logger.Fields{
			field.Error: err,
		})
		return err
	}
	defer disconnectTimer.Stop()
	logger.Info("Connected to TCS endpoint")
	// start a timer and listens for tcs heartbeats/acks. The timer is reset when
	// we receive a heartbeat from the server or when a published metrics message
	// is acked.
	startTime := time.Now()
	heartBeatTimer := newHeartbeatTimeoutHandler(client, startTime, session.heartbeatTimeout, session.heartbeatJitterMax)
	defer heartBeatTimer.Stop()

	client.AddRequestHandler(heartbeatHandler(heartBeatTimer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishMetricHandler(heartBeatTimer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishHealthMetricHandler(heartBeatTimer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishInstanceStatusHandler(heartBeatTimer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.SetAnyRequestHandler(anyMessageHandler(client, wsRWTimeout))
	return client.Serve(ctx)
}

func (session *telemetrySession) getTelemetryEndpoint() (string, error) {
	containerInstanceARN := session.containerInstanceArn
	tcsEndpoint, err := session.ecsClient.DiscoverTelemetryEndpoint(containerInstanceARN)
	if err != nil {
		logger.Error("tcs: unable to discover poll endpoint", logger.Fields{
			field.Error: err,
		})
		return "", err
	}
	return tcsEndpoint, nil
}

// heartbeatHandler resets the heartbeat timer when HeartbeatMessage message is received from tcs.
func heartbeatHandler(timer *time.Timer, heartbeatTimeout, heartbeatJitter time.Duration) func(*ecstcs.HeartbeatMessage) {
	return func(*ecstcs.HeartbeatMessage) {
		logger.Debug("Received HeartbeatMessage from tcs")
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// ackPublishMetricHandler consumes the ack message from the backend. THe backend sends
// the ack each time it processes a metric message.
func ackPublishMetricHandler(timer *time.Timer, heartbeatTimeout, heartbeatJitter time.Duration) func(*ecstcs.AckPublishMetric) {
	return func(*ecstcs.AckPublishMetric) {
		logger.Debug("Received AckPublishMetric from tcs")
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// ackPublishHealthMetricHandler consumes the ack message from backend. The backend sends
// the ack each time it processes a health message
func ackPublishHealthMetricHandler(timer *time.Timer, heartbeatTimeout, heartbeatJitter time.Duration) func(*ecstcs.AckPublishHealth) {
	return func(*ecstcs.AckPublishHealth) {
		logger.Debug("Received ACKPublishHealth from tcs")
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// ackPublishInstanceStatusHandler consumes the ack message from backend. The backend sends
// the ack each time it processes a health message
func ackPublishInstanceStatusHandler(timer *time.Timer, heartbeatTimeout, heartbeatJitter time.Duration) func(*ecstcs.AckPublishInstanceStatus) {
	return func(*ecstcs.AckPublishInstanceStatus) {
		logger.Debug("Received AckPublishInstanceStatus from tcs")
		timer.Reset(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active
func anyMessageHandler(client wsclient.ClientServer, wsRWTimeout time.Duration) func(interface{}) {
	return func(interface{}) {
		logger.Trace("TCS activity occurred")
		// Reset read deadline as there's activity on the channel
		if err := client.SetReadDeadline(time.Now().Add(wsRWTimeout)); err != nil {
			logger.Warn("Unable to extend read deadline for TCS connection", logger.Fields{
				field.Error: err,
			})
		}
	}
}

// newHeartbeatTimeoutHandler returns new timer object to disconnect from tacs server based on connection start time.
func newHeartbeatTimeoutHandler(cs wsclient.ClientServer,
	startTime time.Time,
	heartbeatTimeout time.Duration,
	heartbeatJitter time.Duration,
) *time.Timer {

	maxConnectionDuration := retry.AddJitter(heartbeatTimeout, heartbeatJitter)
	timer := time.AfterFunc(maxConnectionDuration, func() {
		err := cs.CloseClient(startTime, maxConnectionDuration)
		if err != nil {
			logger.Warn(fmt.Sprintf("Attempted disconnecting; tcs client already closed. %s", err))
		}
	})
	return timer
}

// formatURL returns formatted url for tcs endpoint.
func formatURL(endpoint, cluster, containerInstance, agentVersion, agentHash, containerRuntime, containerRuntimeVersion string) string {
	tcsURL := endpoint
	if !strings.HasSuffix(tcsURL, "/") {
		tcsURL += "/"
	}
	query := url.Values{}
	query.Set("cluster", cluster)
	query.Set("containerInstance", containerInstance)
	query.Set("agentVersion", agentVersion)
	query.Set("agentHash", agentHash)
	if containerRuntime == ContainerRuntimeDocker && containerRuntimeVersion != "" {
		query.Set("dockerVersion", containerRuntimeVersion)
	}
	return tcsURL + "ws?" + query.Encode()
}
