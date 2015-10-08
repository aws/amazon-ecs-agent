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

package tcshandler

import (
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const (
	// defaultPublishMetricsInterval is the interval at which utilization
	// metrics from stats engine are published to the backend.
	defaultPublishMetricsInterval = 20 * time.Second

	// The maximum time to wait between heartbeats without disconnecting
	heartbeatTimeout = 5 * time.Minute
	heartbeatJitter  = 3 * time.Minute
)

var log = logger.ForModule("tcs handler")

// StartMetricsSession starts a metric session. It initializes the stats engine
// and invokes StartSession.
func StartMetricsSession(params TelemetrySessionParams) {
	disabled, err := params.isTelemetryDisabled()
	if err != nil {
		log.Warn("Error getting telemetry config", "err", err)
		return
	}

	if !disabled {
		statsEngine := stats.NewDockerStatsEngine(params.Cfg)
		err := statsEngine.MustInit(params.TaskEngine, params.Cfg.Cluster, params.ContainerInstanceArn)
		if err != nil {
			log.Warn("Error initializing metrics engine", "err", err)
			return
		}
		err = StartSession(params, statsEngine)
		if err != nil {
			log.Warn("Error starting metrics session with backend", "err", err)
			return
		}
	} else {
		log.Info("Metric collection disabled")
	}
}

// StartSession creates a session with the backend and handles requests
// using the passed in arguments.
// The engine is expected to initialized and gathering container metrics by
// the time the websocket client starts using it.
func StartSession(params TelemetrySessionParams, statsEngine stats.Engine) error {
	backoff := utils.NewSimpleBackoff(time.Second, 1*time.Minute, 0.2, 2)
	for {
		tcsError := startTelemetrySession(params, statsEngine)
		if tcsError == nil || tcsError == io.EOF {
			backoff.Reset()
		} else {
			log.Info("Error from tcs; backing off", "err", tcsError)
			ttime.Sleep(backoff.Duration())
		}
	}
}

func startTelemetrySession(params TelemetrySessionParams, statsEngine stats.Engine) error {
	tcsEndpoint, err := params.EcsClient.DiscoverTelemetryEndpoint(params.ContainerInstanceArn)
	if err != nil {
		log.Error("Unable to discover poll endpoint", "err", err)
		return err
	}
	log.Debug("Connecting to TCS endpoint " + tcsEndpoint)
	url := formatURL(tcsEndpoint, params.Cfg.Cluster, params.ContainerInstanceArn)
	return startSession(url, params.Cfg.AWSRegion, params.CredentialProvider, params.AcceptInvalidCert, statsEngine, defaultPublishMetricsInterval)
}

func startSession(url string, region string, credentialProvider *credentials.Credentials, acceptInvalidCert bool, statsEngine stats.Engine, publishMetricsInterval time.Duration) error {
	client := tcsclient.New(url, region, credentialProvider, acceptInvalidCert, statsEngine, publishMetricsInterval)

	defer client.Close()

	// start a timer and listens for tcs heartbeats/acks. The timer is reset when
	// we receive a heartbeat from the server or when a publish metrics message
	// is acked.
	timer := time.AfterFunc(utils.AddJitter(heartbeatTimeout, heartbeatJitter), func() {
		// Close the connection if there haven't been any messages received from backend
		// for a long time.
		log.Debug("TCS Connection hasn't had a heartbeat or an ack message in too long of a timeout; disconnecting")
		client.Close()
	})
	defer timer.Stop()
	client.AddRequestHandler(heartbeatHandler(timer))
	client.AddRequestHandler(ackPublishMetricHandler(timer))
	err := client.Connect()
	if err != nil {
		log.Error("Error connecting to TCS: " + err.Error())
		return err
	}
	return client.Serve()
}

// heartbeatHandler resets the heartbeat timer when HeartbeatMessage message is received from tcs.
func heartbeatHandler(timer *time.Timer) func(*ecstcs.HeartbeatMessage) {
	return func(*ecstcs.HeartbeatMessage) {
		log.Debug("Received HeartbeatMessage from tcs")
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// ackPublishMetricHandler consumes the ack message from the backend. THe backend sends
// the ack each time it processes a metric message.
func ackPublishMetricHandler(timer *time.Timer) func(*ecstcs.AckPublishMetric) {
	return func(*ecstcs.AckPublishMetric) {
		log.Debug("Received AckPublishMetric from tcs")
		timer.Reset(utils.AddJitter(heartbeatTimeout, heartbeatJitter))
	}
}

// formatURL returns formatted url for tcs endpoint.
func formatURL(endpoint string, cluster string, containerInstance string) string {
	tcsURL := endpoint
	if !strings.HasSuffix(tcsURL, "/") {
		tcsURL += "/"
	}
	query := url.Values{}
	query.Set("cluster", cluster)
	query.Set("containerInstance", containerInstance)
	return tcsURL + "ws?" + query.Encode()
}
