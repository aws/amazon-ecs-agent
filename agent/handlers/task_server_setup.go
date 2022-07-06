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
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	handlersutils "github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	v4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"
	"github.com/didip/tollbooth"
	"github.com/gorilla/mux"
)

const (
	// readTimeout specifies the maximum duration before timing out read of the request.
	// The value is set to 5 seconds as per AWS SDK defaults.
	readTimeout = 5 * time.Second

	// writeTimeout specifies the maximum duration before timing out write of the response.
	// The value is set to 5 seconds as per AWS SDK defaults.
	writeTimeout = 5 * time.Second
)

func taskServerSetup(credentialsManager credentials.Manager,
	auditLogger audit.AuditLogger,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	cluster string,
	statsEngine stats.Engine,
	steadyStateRate int,
	burstRate int,
	availabilityZone string,
	vpcId string,
	containerInstanceArn string) *http.Server {
	muxRouter := mux.NewRouter()

	// Set this to false so that for request like "//v3//metadata/task"
	// to permanently redirect(301) to "/v3/metadata/task" handler
	muxRouter.SkipClean(false)

	muxRouter.HandleFunc(v1.CredentialsPath,
		v1.CredentialsHandler(credentialsManager, auditLogger))

	v2HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, credentialsManager, auditLogger, availabilityZone, vpcId, containerInstanceArn)

	v3HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, availabilityZone, vpcId, containerInstanceArn)

	v4HandlersSetup(muxRouter, state, ecsClient, statsEngine, cluster, availabilityZone, vpcId, containerInstanceArn)

	limiter := tollbooth.NewLimiter(int64(steadyStateRate), nil)
	limiter.SetOnLimitReached(handlersutils.LimitReachedHandler(auditLogger))
	limiter.SetBurst(burstRate)

	// Log all requests and then pass through to muxRouter.
	loggingMuxRouter := mux.NewRouter()

	// rootPath is a path for any traffic to this endpoint, "root" mux name will not be used.
	rootPath := "/" + handlersutils.ConstructMuxVar("root", handlersutils.AnythingRegEx)
	loggingMuxRouter.Handle(rootPath, tollbooth.LimitHandler(
		limiter, NewLoggingHandler(muxRouter)))

	loggingMuxRouter.SkipClean(false)

	server := http.Server{
		Addr:         "127.0.0.1:" + strconv.Itoa(config.AgentCredentialsPort),
		Handler:      loggingMuxRouter,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return &server
}

// v2HandlersSetup adds all handlers in v2 package to the mux router.
func v2HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	credentialsManager credentials.Manager,
	auditLogger audit.AuditLogger,
	availabilityZone string,
	vpcId string,
	containerInstanceArn string) {
	muxRouter.HandleFunc(v2.CredentialsPath, v2.CredentialsHandler(credentialsManager, auditLogger))
	muxRouter.HandleFunc(v2.ContainerMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskWithTagsMetadataPath, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, true))
	muxRouter.HandleFunc(v2.TaskMetadataPathWithSlash, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, false))
	muxRouter.HandleFunc(v2.TaskWithTagsMetadataPathWithSlash, v2.TaskContainerMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, true))
	muxRouter.HandleFunc(v2.ContainerStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPathWithSlash, v2.TaskContainerStatsHandler(state, statsEngine))
}

// v3HandlersSetup adds all handlers in v3 package to the mux router.
func v3HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	vpcId string,
	containerInstanceArn string) {
	muxRouter.HandleFunc(v3.ContainerMetadataPath, v3.ContainerMetadataHandler(state))
	muxRouter.HandleFunc(v3.TaskMetadataPath, v3.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, false))
	muxRouter.HandleFunc(v3.TaskWithTagsMetadataPath, v3.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, true))
	muxRouter.HandleFunc(v3.ContainerStatsPath, v3.ContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v3.TaskStatsPath, v3.TaskStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v3.ContainerAssociationsPath, v3.ContainerAssociationsHandler(state))
	muxRouter.HandleFunc(v3.ContainerAssociationPathWithSlash, v3.ContainerAssociationHandler(state))
	muxRouter.HandleFunc(v3.ContainerAssociationPath, v3.ContainerAssociationHandler(state))
}

// v4HandlerSetup adda all handlers in v4 package to the mux router
func v4HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	vpcId string,
	containerInstanceArn string) {
	muxRouter.HandleFunc(v4.ContainerMetadataPath, v4.ContainerMetadataHandler(state))
	muxRouter.HandleFunc(v4.TaskMetadataPath, v4.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, false))
	muxRouter.HandleFunc(v4.TaskWithTagsMetadataPath, v4.TaskMetadataHandler(state, ecsClient, cluster, availabilityZone, vpcId, containerInstanceArn, true))
	muxRouter.HandleFunc(v4.ContainerStatsPath, v4.ContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v4.TaskStatsPath, v4.TaskStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v4.ContainerAssociationsPath, v4.ContainerAssociationsHandler(state))
	muxRouter.HandleFunc(v4.ContainerAssociationPathWithSlash, v4.ContainerAssociationHandler(state))
	muxRouter.HandleFunc(v4.ContainerAssociationPath, v4.ContainerAssociationHandler(state))
}

// ServeTaskHTTPEndpoint serves task/container metadata, task/container stats, and IAM Role Credentials
// for tasks being managed by the agent.
func ServeTaskHTTPEndpoint(
	ctx context.Context,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	containerInstanceArn string,
	cfg *config.Config,
	statsEngine stats.Engine,
	availabilityZone string,
	vpcId string) {
	// Create and initialize the audit log
	logger, err := seelog.LoggerFromConfigAsString(audit.AuditLoggerConfig(cfg))
	if err != nil {
		seelog.Errorf("Error initializing the audit log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		logger = seelog.Disabled
	}

	auditLogger := audit.NewAuditLog(containerInstanceArn, cfg, logger)

	server := taskServerSetup(credentialsManager, auditLogger, state, ecsClient, cfg.Cluster, statsEngine,
		cfg.TaskMetadataSteadyStateRate, cfg.TaskMetadataBurstRate, availabilityZone, vpcId, containerInstanceArn)

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
