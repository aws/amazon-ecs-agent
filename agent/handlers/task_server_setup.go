// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"net/http"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	handlersutils "github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/utils"
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
	cluster string,
	statsEngine stats.Engine,
	steadyStateRate int,
	burstRate int) *http.Server {
	muxRouter := mux.NewRouter()

	// Set this so that for request like "/v3//metadata/task", the Agent will pass
	// it to task metadata handler instead of returning a 301 error.
	muxRouter.SkipClean(true)

	muxRouter.HandleFunc(v1.CredentialsPath,
		v1.CredentialsHandler(credentialsManager, auditLogger))

	v2HandlersSetup(muxRouter, state, statsEngine, cluster, credentialsManager, auditLogger)

	v3HandlersSetup(muxRouter, state, statsEngine, cluster)

	limiter := tollbooth.NewLimiter(int64(steadyStateRate), nil)
	limiter.SetOnLimitReached(handlersutils.LimitReachedHandler(auditLogger))
	limiter.SetBurst(burstRate)

	// Log all requests and then pass through to muxRouter.
	loggingMuxRouter := mux.NewRouter()

	// rootPath is a path for any traffic to this endpoint, "root" mux name will not be used.
	rootPath := "/" + handlersutils.ConstructMuxVar("root", handlersutils.AnythingRegEx)
	loggingMuxRouter.Handle(rootPath, tollbooth.LimitHandler(
		limiter, NewLoggingHandler(muxRouter)))

	loggingMuxRouter.SkipClean(true)

	server := http.Server{
		Addr:         ":" + strconv.Itoa(config.AgentCredentialsPort),
		Handler:      loggingMuxRouter,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return &server
}

// v2HandlersSetup adds all handlers in v2 package to the mux router.
func v2HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine,
	cluster string,
	credentialsManager credentials.Manager,
	auditLogger audit.AuditLogger) {
	muxRouter.HandleFunc(v2.CredentialsPath, v2.CredentialsHandler(credentialsManager, auditLogger))
	muxRouter.HandleFunc(v2.ContainerMetadataPath, v2.TaskContainerMetadataHandler(state, cluster))
	muxRouter.HandleFunc(v2.TaskMetadataPath, v2.TaskContainerMetadataHandler(state, cluster))
	muxRouter.HandleFunc(v2.TaskMetadataPathWithSlash, v2.TaskContainerMetadataHandler(state, cluster))
	muxRouter.HandleFunc(v2.ContainerStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPath, v2.TaskContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v2.TaskStatsPathWithSlash, v2.TaskContainerStatsHandler(state, statsEngine))
}

// v3HandlersSetup adds all handlers in v3 package to the mux router.
func v3HandlersSetup(muxRouter *mux.Router,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine,
	cluster string) {
	muxRouter.HandleFunc(v3.ContainerMetadataPath, v3.ContainerMetadataHandler(state))
	muxRouter.HandleFunc(v3.TaskMetadataPath, v3.TaskMetadataHandler(state, cluster))
	muxRouter.HandleFunc(v3.ContainerStatsPath, v3.ContainerStatsHandler(state, statsEngine))
	muxRouter.HandleFunc(v3.TaskStatsPath, v3.TaskStatsHandler(state, statsEngine))
}

// ServeTaskHTTPEndpoint serves task/container metadata, task/container stats, and IAM Role Credentials
// for tasks being managed by the agent.
func ServeTaskHTTPEndpoint(credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	containerInstanceArn string,
	cfg *config.Config,
	statsEngine stats.Engine) {
	// Create and initialize the audit log
	// TODO Use seelog's programmatic configuration instead of xml.
	logger, err := seelog.LoggerFromConfigAsString(audit.AuditLoggerConfig(cfg))
	if err != nil {
		seelog.Errorf("Error initializing the audit log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		logger = seelog.Disabled
	}

	auditLogger := audit.NewAuditLog(containerInstanceArn, cfg, logger)

	server := taskServerSetup(credentialsManager, auditLogger, state, cfg.Cluster, statsEngine,
		cfg.TaskMetadataSteadyStateRate, cfg.TaskMetadataBurstRate)

	for {
		utils.RetryWithBackoff(utils.NewSimpleBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			// TODO, make this cancellable and use the passed in context;
			err := server.ListenAndServe()
			if err != nil {
				seelog.Errorf("Error running http api: %v", err)
			}
			return err
		})
	}
}
