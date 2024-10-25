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

package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	tpfactory "github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/taskprotection"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	v4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	auditinterface "github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds"
	fault "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/handlers"
	faulttype "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	tp "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/taskprotection/v1/handlers"
	tmdsv1 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v1"
	tmdsv2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"

	"github.com/cihub/seelog"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gorilla/mux"
)

const (
	// readTimeout specifies the maximum duration before timing out read of the request.
	// The value is set to 5 seconds as per AWS SDK defaults.
	readTimeout = 5 * time.Second

	// writeTimeout specifies the maximum duration before timing out write of the response.
	// The value is set to 5 seconds as per AWS SDK defaults.
	writeTimeout = 5 * time.Second

	// Timeout for ECS calls. Must be lower than server write timeout defined above.
	ecsCallTimeout = 4 * time.Second
)

func taskServerSetup(
	credentialsManager credentials.Manager,
	auditLogger auditinterface.AuditLogger,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	cluster string,
	statsEngine stats.Engine,
	steadyStateRate int,
	burstRate int,
	availabilityZone string,
	vpcID string,
	containerInstanceArn string,
	taskProtectionClientFactory tp.TaskProtectionClientFactoryInterface,
) (*http.Server, error) {

	muxRouter := mux.NewRouter()

	// Set this to false so that for request like "//v3//metadata/task"
	// to permanently redirect(301) to "/v3/metadata/task" handler
	muxRouter.SkipClean(false)

	muxRouter.HandleFunc(tmdsv1.CredentialsPath,
		tmdsv1.CredentialsHandler(credentialsManager, auditLogger))

	tmdsAgentState := v4.NewTMDSAgentState(state, statsEngine, ecsClient, cluster, availabilityZone, vpcID, containerInstanceArn)
	metricsFactory := metrics.NewNopEntryFactory()

	v2HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, credentialsManager, auditLogger, availabilityZone, containerInstanceArn)

	v3HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, availabilityZone, containerInstanceArn)

	v4HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, availabilityZone, vpcID, containerInstanceArn,
		tmdsAgentState, metricsFactory)

	agentAPIV1HandlersSetup(muxRouter, state, credentialsManager, cluster, tmdsAgentState,
		taskProtectionClientFactory, metricsFactory)

	execWrapper := execwrapper.NewExec()
	registerFaultHandlers(muxRouter, tmdsAgentState, metricsFactory, execWrapper)

	return tmds.NewServer(auditLogger,
		tmds.WithHandler(muxRouter),
		tmds.WithListenAddress(tmds.AddressIPv4()),
		tmds.WithReadTimeout(readTimeout),
		tmds.WithWriteTimeout(writeTimeout),
		tmds.WithSteadyStateRate(float64(steadyStateRate)),
		tmds.WithBurstRate(burstRate))
}

// v2HandlersSetup adds all handlers in v2 package to the mux router.
func v2HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	credentialsManager credentials.Manager,
	auditLogger auditinterface.AuditLogger,
	availabilityZone string,
	containerInstanceArn string) {
	muxRouter.HandleFunc(tmdsv2.CredentialsPath, tmdsv2.CredentialsHandler(credentialsManager, auditLogger))
	muxRouter.HandleFunc(v2.ContainerMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskWithTagsMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, true))
	muxRouter.HandleFunc(v2.TaskMetadataPathWithSlash, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskWithTagsMetadataPathWithSlash, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, true))
	muxRouter.HandleFunc(v2.ContainerStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPathWithSlash, v2.TaskContainerStatsHandler(state, statsEngine))
}

// v3HandlersSetup adds all handlers in v3 package to the mux router.
func v3HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	containerInstanceArn string) {
	muxRouter.HandleFunc(v3.ContainerMetadataPath, v3.ContainerMetadataHandler(state))
	muxRouter.HandleFunc(v3.TaskMetadataPath, v3.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, false))
	muxRouter.HandleFunc(v3.TaskWithTagsMetadataPath, v3.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, containerInstanceArn, true))
	muxRouter.HandleFunc(v3.ContainerStatsPath, v3.ContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v3.TaskStatsPath, v3.TaskStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v3.ContainerAssociationsPath, v3.ContainerAssociationsHandler(state))
	muxRouter.HandleFunc(v3.ContainerAssociationPathWithSlash, v3.ContainerAssociationHandler(state))
	muxRouter.HandleFunc(v3.ContainerAssociationPath, v3.ContainerAssociationHandler(state))
}

// v4HandlerSetup adda all handlers in v4 package to the mux router
func v4HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	vpcID string,
	containerInstanceArn string,
	tmdsAgentState *v4.TMDSAgentState,
	metricsFactory metrics.EntryFactory,
) {
	muxRouter.HandleFunc(tmdsv4.ContainerMetadataPath(), tmdsv4.ContainerMetadataHandler(tmdsAgentState, metricsFactory))
	muxRouter.HandleFunc(tmdsv4.TaskMetadataPath(), tmdsv4.TaskMetadataHandler(tmdsAgentState, metricsFactory))
	muxRouter.HandleFunc(tmdsv4.TaskMetadataWithTagsPath(), tmdsv4.TaskMetadataWithTagsHandler(tmdsAgentState, metricsFactory))
	muxRouter.HandleFunc(tmdsv4.ContainerStatsPath(), tmdsv4.ContainerStatsHandler(tmdsAgentState, metricsFactory))
	muxRouter.HandleFunc(tmdsv4.TaskStatsPath(), tmdsv4.TaskStatsHandler(tmdsAgentState, metricsFactory))
	muxRouter.HandleFunc(v4.ContainerAssociationsPath, v4.ContainerAssociationsHandler(state))
	muxRouter.HandleFunc(v4.ContainerAssociationPathWithSlash, v4.ContainerAssociationHandler(state))
	muxRouter.HandleFunc(v4.ContainerAssociationPath, v4.ContainerAssociationHandler(state))
}

// agentAPIV1HandlersSetup adds handlers for Agent API V1
func agentAPIV1HandlersSetup(
	muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	credentialsManager credentials.Manager,
	cluster string,
	agentState *v4.TMDSAgentState,
	factory tp.TaskProtectionClientFactoryInterface,
	metricsFactory metrics.EntryFactory,
) {
	muxRouter.
		HandleFunc(
			tp.TaskProtectionPath(),
			tp.UpdateTaskProtectionHandler(agentState, credentialsManager,
				factory, cluster, metricsFactory, ecsCallTimeout)).
		Methods("PUT")
	muxRouter.
		HandleFunc(
			tp.TaskProtectionPath(),
			tp.GetTaskProtectionHandler(agentState, credentialsManager,
				factory, cluster, metricsFactory, ecsCallTimeout)).
		Methods("GET")
}

// registerFaultHandlers adds handlers for fault endpoints
// TODO: Will need to be called in taskServerSetup once all of the handlers have been implemented
func registerFaultHandlers(
	muxRouter *mux.Router,
	agentState *v4.TMDSAgentState,
	metricsFactory metrics.EntryFactory,
	execWrapper execwrapper.Exec,
) {
	handler := fault.New(agentState, metricsFactory, execWrapper)

	if muxRouter == nil {
		return
	}

	// Setting up handler endpoints for network blackhole port fault injections
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.BlackHolePortFaultType, faulttype.StartNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StartNetworkBlackholePort(),
			),
			metricsFactory,
			faulttype.StartNetworkFaultPostfix,
			faulttype.BlackHolePortFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.BlackHolePortFaultType, faulttype.StopNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StopNetworkBlackHolePort(),
			),
			metricsFactory,
			faulttype.StopNetworkFaultPostfix,
			faulttype.BlackHolePortFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.BlackHolePortFaultType, faulttype.CheckNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.CheckNetworkBlackHolePort(),
			),
			metricsFactory,
			faulttype.CheckNetworkFaultPostfix,
			faulttype.BlackHolePortFaultType,
		),
	).Methods("POST")

	// Setting up handler endpoints for network latency fault injections
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.LatencyFaultType, faulttype.StartNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StartNetworkLatency(),
			),
			metricsFactory,
			faulttype.StartNetworkFaultPostfix,
			faulttype.LatencyFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.LatencyFaultType, faulttype.StopNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StopNetworkLatency(),
			),
			metricsFactory,
			faulttype.StopNetworkFaultPostfix,
			faulttype.LatencyFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.LatencyFaultType, faulttype.CheckNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.CheckNetworkLatency(),
			),
			metricsFactory,
			faulttype.CheckNetworkFaultPostfix,
			faulttype.LatencyFaultType,
		),
	).Methods("POST")

	// Setting up handler endpoints for network packet loss fault injections
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.PacketLossFaultType, faulttype.StartNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StartNetworkPacketLoss(),
			),
			metricsFactory,
			faulttype.StartNetworkFaultPostfix,
			faulttype.PacketLossFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.PacketLossFaultType, faulttype.StopNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.StopNetworkPacketLoss(),
			),
			metricsFactory,
			faulttype.StopNetworkFaultPostfix,
			faulttype.PacketLossFaultType,
		),
	).Methods("POST")
	muxRouter.Handle(
		fault.NetworkFaultPath(faulttype.PacketLossFaultType, faulttype.CheckNetworkFaultPostfix),
		fault.TelemetryMiddleware(
			tollbooth.LimitFuncHandler(
				createRateLimiter(),
				handler.CheckNetworkPacketLoss(),
			),
			metricsFactory,
			faulttype.CheckNetworkFaultPostfix,
			faulttype.PacketLossFaultType,
		),
	).Methods("POST")

	seelog.Debug("Successfully set up Fault TMDS handlers")
}

// Creates a tollbooth ratelimiter for the Fault Handler APIs
func createRateLimiter() *limiter.Limiter {
	lmt := tollbooth.NewLimiter(0.2, nil)
	lmt.SetMessage("You have reached maximum request limit")
	return lmt
}

// ServeTaskHTTPEndpoint serves task/container metadata, task/container stats, IAM Role Credentials, and Agent APIs
// for tasks being managed by the agent.
func ServeTaskHTTPEndpoint(
	ctx context.Context,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	containerInstanceArn string,
	cfg *config.Config,
	statsEngine stats.Engine,
	availabilityZone string,
	vpcID string) {
	// Create and initialize the audit log
	logger, err := seelog.LoggerFromConfigAsString(audit.AuditLoggerConfig(cfg))
	if err != nil {
		seelog.Errorf("Error initializing the audit log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		logger = seelog.Disabled
	}

	auditLogger := audit.NewAuditLog(containerInstanceArn, cfg, logger)

	taskProtectionClientFactory := tpfactory.TaskProtectionClientFactory{
		Region: cfg.AWSRegion, Endpoint: cfg.APIEndpoint, AcceptInsecureCert: cfg.AcceptInsecureCert,
	}
	server, err := taskServerSetup(credentialsManager, auditLogger, state, ecsClient, cfg.Cluster,
		statsEngine, cfg.TaskMetadataSteadyStateRate, cfg.TaskMetadataBurstRate,
		availabilityZone, vpcID, containerInstanceArn, taskProtectionClientFactory)
	if err != nil {
		seelog.Criticalf("Failed to set up Task Metadata Server: %v", err)
		return
	}

	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			seelog.Infof("HTTP server Shutdown: %v", err)
		}
	}()

	for {
		retry.RetryWithBackoff(retry.NewExponentialBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				seelog.Errorf("Error running task api: %v", err)
				return err
			}
			// server was cleanly closed via context
			return nil
		})
	}
}
