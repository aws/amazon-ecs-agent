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
	"github.com/cihub/seelog"
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
	doctor *doctor.Doctor) *DockerTelemetrySession {
	ok, cfgParseErr := isContainerHealthMetricsDisabled(cfg)
	if cfgParseErr != nil {
		seelog.Warnf("Error starting metrics session: %v", cfgParseErr)
		return nil
	}
	if ok {
		seelog.Warnf("Metrics were disabled, not starting the telemetry session")
		return nil
	}

	agentVersion, agentHash, containerRuntimeVersion := generateVersionInfo(taskEngine)
	if cfg == nil {
		logger.Error("Config is empty in the tcs session parameter")
		return nil
	}

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
	return &DockerTelemetrySession{session, ecsClient, containerInstanceArn}
}

// Start "overloads" tcshandler.TelemetrySession's Start with extra handling of discoverTelemetryEndpoint result.
// discoverTelemetryEndpoint and tcshandler.TelemetrySession's StartTelemetrySession errors are handled
// (retryWithBackoff or return) in a combined manner
func (session *DockerTelemetrySession) Start(ctx context.Context) error {
	backoff := retry.NewExponentialBackoff(time.Second, 1*time.Minute, 0.2, 2)
	for {
		endpoint, tcsError := discoverPollEndpoint(session.containerInstanceArn, session.ecsClient)
		if tcsError == nil {
			tcsError = session.s.StartTelemetrySession(ctx, endpoint)
		}
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
