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
)

type DockerTelemetrySession struct {
	s tcshandler.TelemetrySession
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
		ecsClient,
	)
	return &DockerTelemetrySession{session}, nil
}

func (session *DockerTelemetrySession) Start(ctx context.Context) error {
	return session.s.Start(ctx)
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

func isContainerHealthMetricsDisabled(cfg *config.Config) (bool, error) {
	if cfg != nil {
		return cfg.DisableMetrics.Enabled() && cfg.DisableDockerHealthCheck.Enabled(), nil
	}
	return false, errors.New("config is empty in the tcs session parameter")
}
