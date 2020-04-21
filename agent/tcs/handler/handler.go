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
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	tcsclient "github.com/aws/amazon-ecs-agent/agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	// The maximum time to wait between heartbeats without disconnecting
	defaultHeartbeatTimeout = 1 * time.Minute
	defaultHeartbeatJitter  = 1 * time.Minute
	// wsRWTimeout is the duration of read and write deadline for the
	// websocket connection
	wsRWTimeout                        = 2*defaultHeartbeatTimeout + defaultHeartbeatJitter
	deregisterContainerInstanceHandler = "TCSDeregisterContainerInstanceHandler"
)

// StartMetricsSession starts a metric session. It initializes the stats engine
// and invokes StartSession.
func StartMetricsSession(params *TelemetrySessionParams) {
	ok, err := params.isContainerHealthMetricsDisabled()
	if err != nil {
		seelog.Warnf("Error starting metrics session: %v", err)
		return
	}
	if ok {
		seelog.Warnf("Metrics were disabled, not starting the telemetry session")
		return
	}

	err = params.StatsEngine.MustInit(params.Ctx, params.TaskEngine, params.Cfg.Cluster,
		params.ContainerInstanceArn)
	if err != nil {
		seelog.Warnf("Error initializing metrics engine: %v", err)
		return
	}

	err = StartSession(params, params.StatsEngine)
	if err != nil {
		seelog.Warnf("Error starting metrics session with backend: %v", err)
	}
}

// StartSession creates a session with the backend and handles requests
// using the passed in arguments.
// The engine is expected to initialized and gathering container metrics by
// the time the websocket client starts using it.
func StartSession(params *TelemetrySessionParams, statsEngine stats.Engine) error {
	backoff := retry.NewExponentialBackoff(time.Second, 1*time.Minute, 0.2, 2)
	for {
		tcsError := startTelemetrySession(params, statsEngine)
		if tcsError == nil || tcsError == io.EOF {
			seelog.Info("TCS Websocket connection closed for a valid reason")
			backoff.Reset()
		} else {
			seelog.Errorf("Error: lost websocket connection with ECS Telemetry service (TCS): %v", tcsError)
			params.time().Sleep(backoff.Duration())
		}
		select {
		case <-params.Ctx.Done():
			seelog.Info("TCS session exited cleanly.")
			return nil
		default:
		}
	}
}

func startTelemetrySession(params *TelemetrySessionParams, statsEngine stats.Engine) error {
	tcsEndpoint, err := params.ECSClient.DiscoverTelemetryEndpoint(params.ContainerInstanceArn)
	if err != nil {
		seelog.Errorf("tcs: unable to discover poll endpoint: %v", err)
		return err
	}
	url := formatURL(tcsEndpoint, params.Cfg.Cluster, params.ContainerInstanceArn, params.TaskEngine)
	return startSession(params.Ctx, url, params.Cfg, params.CredentialProvider, statsEngine,
		defaultHeartbeatTimeout, defaultHeartbeatJitter, config.DefaultContainerMetricsPublishInterval,
		config.DefaultInstanceHealthMetricsPublishInterval, params.DeregisterInstanceEventStream)
}

func startSession(
	ctx context.Context,
	url string,
	cfg *config.Config,
	credentialProvider *credentials.Credentials,
	statsEngine stats.Engine,
	heartbeatTimeout, heartbeatJitter,
	publishMetricsInterval time.Duration,
	publishInstanceHealthMetricsInterval time.Duration,
	deregisterInstanceEventStream *eventstream.EventStream) error {
	client := tcsclient.New(url, cfg, credentialProvider, statsEngine,
		publishMetricsInterval, publishInstanceHealthMetricsInterval, wsRWTimeout, cfg.DisableMetrics)
	defer client.Close()

	err := deregisterInstanceEventStream.Subscribe(deregisterContainerInstanceHandler, client.Disconnect)
	if err != nil {
		return err
	}
	defer deregisterInstanceEventStream.Unsubscribe(deregisterContainerInstanceHandler)

	err = client.Connect()
	if err != nil {
		seelog.Errorf("Error connecting to TCS: %v", err.Error())
		return err
	}
	seelog.Info("Connected to TCS endpoint")
	// start a timer and listens for tcs heartbeats/acks. The timer is reset when
	// we receive a heartbeat from the server or when a publish metrics message
	// is acked.
	timer := time.NewTimer(retry.AddJitter(heartbeatTimeout, heartbeatJitter))
	defer timer.Stop()
	client.AddRequestHandler(heartbeatHandler(timer))
	client.AddRequestHandler(ackPublishMetricHandler(timer))
	client.AddRequestHandler(ackPublishHealthMetricHandler(timer))
	client.AddRequestHandler(ackPublishInstanceHealthMetricHandler(timer))
	client.SetAnyRequestHandler(anyMessageHandler(client))
	serveC := make(chan error)
	go func() {
		serveC <- client.Serve()
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
func heartbeatHandler(timer *time.Timer) func(*ecstcs.HeartbeatMessage) {
	return func(*ecstcs.HeartbeatMessage) {
		seelog.Debug("Received HeartbeatMessage from tcs")
		timer.Reset(retry.AddJitter(defaultHeartbeatTimeout, defaultHeartbeatJitter))
	}
}

// ackPublishMetricHandler consumes the ack message from the backend. THe backend sends
// the ack each time it processes a metric message.
func ackPublishMetricHandler(timer *time.Timer) func(*ecstcs.AckPublishMetric) {
	return func(*ecstcs.AckPublishMetric) {
		seelog.Debug("Received AckPublishMetric from tcs")
		timer.Reset(retry.AddJitter(defaultHeartbeatTimeout, defaultHeartbeatJitter))
	}
}

// ackPublishHealthMetricHandler consumes the ack message from backend. The backend sends
// the ack each time it processes a health message
func ackPublishHealthMetricHandler(timer *time.Timer) func(*ecstcs.AckPublishHealth) {
	return func(*ecstcs.AckPublishHealth) {
		seelog.Debug("Received ACKPublishHealth from tcs")
		timer.Reset(retry.AddJitter(defaultHeartbeatTimeout, defaultHeartbeatJitter))
	}
}

// ackPublishInstanceHealthMetricHandler consumes the ack message from backend. The backend sends
// the ack each time it processes a instance health message
func ackPublishInstanceHealthMetricHandler(timer *time.Timer) func(*ecstcs.AckPublishInstanceHealth) {
	return func(*ecstcs.AckPublishInstanceHealth) {
		seelog.Debug("Received ACKPublishInstanceHealth from tcs")
		timer.Reset(retry.AddJitter(defaultHeartbeatTimeout, defaultHeartbeatJitter))
	}
}

// anyMessageHandler handles any server message. Any server message means the
// connection is active
func anyMessageHandler(client wsclient.ClientServer) func(interface{}) {
	return func(interface{}) {
		seelog.Trace("TCS activity occurred")
		// Reset read deadline as there's activity on the channel
		if err := client.SetReadDeadline(time.Now().Add(wsRWTimeout)); err != nil {
			seelog.Warnf("Unable to extend read deadline for TCS connection: %v", err)
		}
	}
}

// formatURL returns formatted url for tcs endpoint.
func formatURL(endpoint string, cluster string, containerInstance string, taskEngine engine.TaskEngine) string {
	tcsURL := endpoint
	if !strings.HasSuffix(tcsURL, "/") {
		tcsURL += "/"
	}
	query := url.Values{}
	query.Set("cluster", cluster)
	query.Set("containerInstance", containerInstance)
	query.Set("agentVersion", version.Version)
	query.Set("agentHash", version.GitHashString())
	if dockerVersion, err := taskEngine.Version(); err == nil {
		query.Set("dockerVersion", dockerVersion)
	}
	return tcsURL + "ws?" + query.Encode()
}
