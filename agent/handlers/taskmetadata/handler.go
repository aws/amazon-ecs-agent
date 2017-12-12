// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package taskmetadata

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	log "github.com/cihub/seelog"
	"github.com/didip/tollbooth"
)

const (
	// readTimeout specifies the maximum duration before timing out read of the request.
	// The value is set to 5 seconds as per AWS SDK defaults.
	readTimeout = 5 * time.Second
	// writeTimeout specifies the maximum duration before timing out write of the response.
	// The value is set to 5 seconds as per AWS SDK defaults.
	writeTimeout = 5 * time.Second

	// Credentials API versions
	apiVersion1 = 1
	apiVersion2 = 2

	// Rate limits for the metadata endpoint(s) and APIs by request ip addresses. Today, we
	// support the following endpoints:
	// 1. /v2/credentials: For serving IAM role credentials
	// 2. /v2/metadata: For serving task and container metadata information (for "awsvpc" tasks)
	// 3. /v2/stats: For serving task and container stats information (for "awsvpc" tasks)

	// rateLimitPerSecond specifies the steady state throttle for the task metadata endpoint.
	// Because all containers in a task share the same IP address in an "awsvpc" task, and a
	// task can be constituted of up to 10 containers, the steady state rate is set at 10
	// per second
	rateLimitPerSecond = 10

	// rateLimitBurstPerSecond specifies the burst rate throttle for the task metadata endpoint.
	rateLimitBurstPerSecond = 15
)

// ServeHTTP serves IAM Role Credentials for Tasks being managed by the agent.
func ServeHTTP(credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	containerInstanceArn string,
	cfg *config.Config,
	statsEngine stats.Engine) {
	// Create and initialize the audit log
	// TODO Use seelog's programmatic configuration instead of xml.
	logger, err := log.LoggerFromConfigAsString(audit.AuditLoggerConfig(cfg))
	if err != nil {
		log.Errorf("Error initializing the audit log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		logger = log.Disabled
	}

	auditLogger := audit.NewAuditLog(containerInstanceArn, cfg, logger)

	server := setupServer(credentialsManager, auditLogger, state, cfg.Cluster, statsEngine)

	for {
		utils.RetryWithBackoff(utils.NewSimpleBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			// TODO, make this cancellable and use the passed in context;
			err := server.ListenAndServe()
			if err != nil {
				log.Errorf("Error running http api: %v", err)
			}
			return err
		})
	}
}

// setupServer starts the HTTP server for serving IAM Role Credentials for Tasks.
func setupServer(credentialsManager credentials.Manager,
	auditLogger audit.AuditLogger,
	state dockerstate.TaskEngineState,
	cluster string,
	statsEngine stats.Engine) *http.Server {
	serverMux := http.NewServeMux()
	// Credentials handlers
	serverMux.HandleFunc(credentials.V1CredentialsPath,
		credentialsV1V2RequestHandler(
			credentialsManager, auditLogger, getV1CredentialsID, apiVersion1))
	serverMux.HandleFunc(credentials.V2CredentialsPath+"/",
		credentialsV1V2RequestHandler(
			credentialsManager, auditLogger, getV2CredentialsID, apiVersion2))
	// Metadata handlers
	serverMux.HandleFunc(metadataPath+"/", metadataV2Handler(state, cluster))
	serverMux.HandleFunc(metadataPath, metadataV2Handler(state, cluster))
	// Stats handlers
	serverMux.HandleFunc(statsPath+"/", statsV2Handler(state, statsEngine))
	serverMux.HandleFunc(statsPath, statsV2Handler(state, statsEngine))

	limiter := tollbooth.NewLimiter(rateLimitPerSecond, nil)
	limiter.SetBurst(rateLimitBurstPerSecond)
	// Log all requests and then pass through to serverMux
	loggingServeMux := http.NewServeMux()
	loggingServeMux.Handle("/", tollbooth.LimitHandler(
		limiter, handlers.NewLoggingHandler(serverMux)))

	server := http.Server{
		Addr:         ":" + strconv.Itoa(config.AgentCredentialsPort),
		Handler:      loggingServeMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return &server
}
