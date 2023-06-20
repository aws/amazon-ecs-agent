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

//lint:file-ignore U1000 Ignore unused metricsFactory field as it is only used by Fargate

package tcshandler

import (
	"context"
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
)

// TelemetrySession defines an interface for handler's long-lived connection with TCS.
type TelemetrySession interface {
	StartTelemetrySession(context.Context, string) error
	Start(context.Context) error
}

// telemetrySession is the base session params type which contains all the parameters required to start a tcs session
type telemetrySession struct {
	containerInstanceArn          string
	cluster                       string
	agentVersion                  string
	agentHash                     string
	containerRuntimeVersion       string
	endpoint                      string
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
}

func NewTelemetrySession(
	containerInstanceArn string,
	cluster string,
	agentVersion string,
	agentHash string,
	containerRuntimeVersion string,
	endpoint string,
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
) TelemetrySession {
	return &telemetrySession{
		containerInstanceArn:          containerInstanceArn,
		cluster:                       cluster,
		agentVersion:                  agentVersion,
		agentHash:                     agentHash,
		containerRuntimeVersion:       containerRuntimeVersion,
		endpoint:                      endpoint,
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
	}
}

// Start runs in for loop to start telemetry session with exponential backoff
func (session *telemetrySession) Start(ctx context.Context) error {
	backoff := retry.NewExponentialBackoff(time.Second, 1*time.Minute, 0.2, 2)
	for {
		tcsError := session.StartTelemetrySession(ctx, session.endpoint)
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
func (session *telemetrySession) StartTelemetrySession(ctx context.Context, endpoint string) error {
	wsRWTimeout := 2*session.heartbeatTimeout + session.heartbeatJitterMax

	var containerRuntime string
	if session.cfg.IsDocker {
		containerRuntime = ContainerRuntimeDocker
	}

	tcsEndpointUrl := formatURL(endpoint, session.cluster, session.containerInstanceArn, session.agentVersion,
		session.agentHash, containerRuntime, session.containerRuntimeVersion)
	client := tcsclient.New(tcsEndpointUrl, session.cfg, session.doctor, session.disableMetrics, tcsclient.DefaultContainerMetricsPublishInterval,
		session.credentialsProvider, wsRWTimeout, session.metricsChannel, session.healthChannel)
	defer client.Close()

	if session.deregisterInstanceEventStream != nil {
		err := session.deregisterInstanceEventStream.Subscribe(deregisterContainerInstanceHandler, client.Disconnect)
		if err != nil {
			return err
		}
		defer session.deregisterInstanceEventStream.Unsubscribe(deregisterContainerInstanceHandler)
	}
	err := client.Connect()
	if err != nil {
		logger.Error("Error connecting to TCS", logger.Fields{
			field.Error: err,
		})
		return err
	}
	logger.Info("Connected to TCS endpoint")
	// start a timer and listens for tcs heartbeats/acks. The timer is reset when
	// we receive a heartbeat from the server or when a published metrics message
	// is acked.
	timer := time.NewTimer(retry.AddJitter(session.heartbeatTimeout, session.heartbeatJitterMax))
	defer timer.Stop()
	client.AddRequestHandler(heartbeatHandler(timer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishMetricHandler(timer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishHealthMetricHandler(timer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.AddRequestHandler(ackPublishInstanceStatusHandler(timer, session.heartbeatTimeout, session.heartbeatJitterMax))
	client.SetAnyRequestHandler(anyMessageHandler(client, wsRWTimeout))
	serveC := make(chan error, 1)
	go func() {
		serveC <- client.Serve(ctx)
	}()
	select {
	case <-ctx.Done():
		// outer context done, agent is exiting
		client.Disconnect()
	case <-timer.C:
		seelog.Info("TCS Connection hasn't had any activity for too long; disconnecting")
		client.Disconnect()
	case err := <-serveC:
		return err
	}
	return nil
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
