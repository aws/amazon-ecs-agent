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
	"github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit/request"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
	"github.com/didip/tollbooth"
)

const (
	// readTimeout specifies the maximum duration before timing out read of the request.
	// The value is set to 5 seconds as per AWS SDK defaults.
	readTimeout = 5 * time.Second
	// writeTimeout specifies the maximum duration before timing out write of the response.
	// The value is set to 5 seconds as per AWS SDK defaults.
	writeTimeout = 5 * time.Second
)

// v2SetupServer creates the HTTP server for serving task/container metadata,
// task/container stats and IAM Role credentials for tasks,
func v2SetupServer(credentialsManager credentials.Manager,
	auditLogger audit.AuditLogger,
	state dockerstate.TaskEngineState,
	cluster string,
	statsEngine stats.Engine,
	steadyStateRate int,
	burstRate int) *http.Server {
	serverMux := http.NewServeMux()
	// Credentials handlers
	serverMux.HandleFunc(credentials.V1CredentialsPath,
		v1.CredentialsHandler(credentialsManager, auditLogger))
	serverMux.HandleFunc(credentials.V2CredentialsPath+"/",
		v2.CredentialsHandler(credentialsManager, auditLogger))
	// Metadata handlers
	serverMux.HandleFunc(v2.TaskContainerMetadataPath+"/", v2.TaskContainerMetadataHandler(state, cluster))
	serverMux.HandleFunc(v2.TaskContainerMetadataPath, v2.TaskContainerMetadataHandler(state, cluster))
	// Stats handlers
	serverMux.HandleFunc(v2.StatsPath+"/", v2.StatsHandler(state, statsEngine))
	serverMux.HandleFunc(v2.StatsPath, v2.StatsHandler(state, statsEngine))

	limiter := tollbooth.NewLimiter(int64(steadyStateRate), nil)
	limiter.SetOnLimitReached(limitReachedHandler(auditLogger))
	limiter.SetBurst(burstRate)
	// Log all requests and then pass through to serverMux
	loggingServeMux := http.NewServeMux()

	loggingServeMux.Handle("/", tollbooth.LimitHandler(
		limiter, NewLoggingHandler(serverMux)))

	server := http.Server{
		Addr:         ":" + strconv.Itoa(config.AgentCredentialsPort),
		Handler:      loggingServeMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return &server
}

// limitReachedHandler logs the throttled request in the credentials audit log
func limitReachedHandler(auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logRequest := request.LogRequest{
			Request: r,
		}
		auditLogger.Log(logRequest, http.StatusTooManyRequests, "")
	}
}

// V2ServeHTTP serves task/container metadata, task/container stats, and IAM Role Credentials
// for Tasks being managed by the agent.
func V2ServeHTTP(credentialsManager credentials.Manager,
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

	server := v2SetupServer(credentialsManager, auditLogger, state, cfg.Cluster, statsEngine,
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
