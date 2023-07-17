// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package reporter

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	tcshandler "github.com/aws/amazon-ecs-agent/ecs-agent/tcs/handler"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const (
	// The maximum time to wait between heartbeats without disconnecting
	defaultHeartbeatTimeout = 1 * time.Minute
	defaultHeartbeatJitter  = 1 * time.Minute
	// Default websocket client disconnection timeout initiated by agent
	defaultDisconnectionTimeout = 15 * time.Minute
	defaultDisconnectionJitter  = 30 * time.Minute
	backoffMin                  = 1 * time.Second
	backoffMax                  = 1 * time.Minute
	jitterMultiple              = 0.2
	multiple                    = 2
)

type DockerTelemetrySession struct {
	s                    tcshandler.TelemetrySession
	ecsClient            api.ECSClient
	containerInstanceArn string
}

// NewDockerTelemetrySession returns creates a DockerTelemetrySession, which has a tcshandler.TelemetrySession embedded.
// tcshandler.TelemetrySession contains the logic to manage the TCSClient and corresponding websocket connection
func NewDockerTelemetrySession(
	containerInstanceArn string,
	credentialProvider *credentials.Credentials,
	cfg *config.Config,
	deregisterInstanceEventStream *eventstream.EventStream,
	ecsClient api.ECSClient,
	taskEngine engine.TaskEngine,
	metricsChannel <-chan ecstcs.TelemetryMessage,
	healthChannel <-chan ecstcs.HealthMessage,
	doctor *doctor.Doctor) (*DockerTelemetrySession, error) {
	ok, cfgParseErr := isContainerHealthMetricsDisabled(cfg)
	if cfgParseErr != nil {
		logger.Warn("Error starting metrics session", logger.Fields{
			field.Error: cfgParseErr,
		})
		return nil, cfgParseErr
	}
	if ok {
		logger.Warn("Both metrics and health were disabled, but still starting the telemetry session for service connect metrics")
	}

	agentVersion, agentHash, containerRuntimeVersion := generateVersionInfo(taskEngine)

	session := tcshandler.NewTelemetrySession(
		containerInstanceArn,
		cfg.Cluster,
		agentVersion,
		agentHash,
		containerRuntimeVersion,
		"", // this will be overridden by DockerTelemetrySession.Start()
		cfg.DisableMetrics.Enabled(),
		credentialProvider,
		&wsclient.WSClientMinAgentConfig{
			AWSRegion:          cfg.AWSRegion,
			AcceptInsecureCert: cfg.AcceptInsecureCert,
			DockerEndpoint:     cfg.DockerEndpoint,
			IsDocker:           true,
		},
		deregisterInstanceEventStream,
		defaultHeartbeatTimeout,
		defaultHeartbeatJitter,
		defaultDisconnectionTimeout,
		defaultDisconnectionJitter,
		nil,
		metricsChannel,
		healthChannel,
		doctor,
	)
	return &DockerTelemetrySession{session, ecsClient, containerInstanceArn}, nil
}

// Start "overloads" tcshandler.TelemetrySession's Start with extra handling of discoverTelemetryEndpoint result.
// discoverTelemetryEndpoint and tcshandler.TelemetrySession's StartTelemetrySession errors are handled
// (retryWithBackoff or return) in a combined manner
func (session *DockerTelemetrySession) Start(ctx context.Context) error {
	backoff := retry.NewExponentialBackoff(backoffMin, backoffMax, jitterMultiple, multiple)
	for {
		select {
		case <-ctx.Done():
			logger.Info("ECS Telemetry service (TCS) session exited cleanly.")
			return nil
		default:
		}
		endpoint, tcsError := discoverPollEndpoint(session.containerInstanceArn, session.ecsClient)
		if tcsError == nil {
			// returning from StartTelemetrySession indicates a disconnection, need to reconnect.
			tcsError = session.s.StartTelemetrySession(ctx, endpoint)
		}
		if tcsError == nil || tcsError == io.EOF {
			// reset backoff when TCS closed for a valid reason, such as connection expiring due to inactivity
			logger.Info("TCS Websocket connection closed for a valid reason")
			backoff.Reset()
		} else {
			// backoff when there is unexpected error, such as invalid frame sent through connection.
			logger.Error("Error: lost websocket connection with ECS Telemetry service (TCS)", logger.Fields{
				field.Error: tcsError,
			})
			time.Sleep(backoff.Duration())
		}
	}
}

// generateVersionInfo generates the agentVersion, agentHash and containerRuntimeVersion from dockerTaskEngine state
func generateVersionInfo(taskEngine engine.TaskEngine) (string, string, string) {
	agentVersion := version.Version
	agentHash := version.GitHashString()
	var containerRuntimeVersion string
	if dockerVersion, getVersionErr := taskEngine.Version(); getVersionErr == nil {
		containerRuntimeVersion = dockerVersion
	}

	return agentVersion, agentHash, containerRuntimeVersion
}

// discoverPollEndpoint calls DiscoverTelemetryEndpoint to get the TCS endpoint url for TCS client to connect
func discoverPollEndpoint(containerInstanceArn string, ecsClient api.ECSClient) (string, error) {
	tcsEndpoint, err := ecsClient.DiscoverTelemetryEndpoint(containerInstanceArn)
	if err != nil {
		logger.Error("tcs: unable to discover poll endpoint", logger.Fields{
			field.Error: err,
		})
	}
	return tcsEndpoint, err
}

func isContainerHealthMetricsDisabled(cfg *config.Config) (bool, error) {
	if cfg != nil {
		return cfg.DisableMetrics.Enabled() && cfg.DisableDockerHealthCheck.Enabled(), nil
	}
	return false, errors.New("config is empty in the tcs session parameter")
}
