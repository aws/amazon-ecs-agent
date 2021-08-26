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

package app

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/app/args"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws"
	log "github.com/cihub/seelog"
)

const (
	ECS_AGENT_HEALTHCHECK_HOST_ENV_VAR = "ECS_AGENT_HEALTHCHECK_HOST"
)

// Run runs the ECS Agent App. It returns an exit code, which is used by
// main() to set the status code for the program
func Run(arguments []string) int {
	defer log.Flush()

	parsedArgs, err := args.New(arguments)
	if err != nil {
		return exitcodes.ExitTerminal
	}

	if *parsedArgs.License {
		return printLicense()
	} else if *parsedArgs.Version {
		return version.PrintVersion()
	} else if *parsedArgs.Healthcheck {
		// Timeout is purposely set to shorter than the default docker healthcheck
		// timeout of 30s. This is so that we can catch any http timeout and log the
		// issue within agent logs.
		// see https://docs.docker.com/engine/reference/builder/#healthcheck
		// if ECS_AGENT_HEALTHCHECK_HOST env var is set, it will override
		// the healthcheck's localhost ip address inside the ecs-agent container
		localhost := "localhost"
		if localhostOverride := os.Getenv(ECS_AGENT_HEALTHCHECK_HOST_ENV_VAR); localhostOverride != "" {
			localhost = localhostOverride
		}
		healthcheckUrl := fmt.Sprintf("http://%s:51678/v1/metadata", localhost)
		return runHealthcheck(healthcheckUrl, time.Second*25)
	}

	if *parsedArgs.LogLevel != "" {
		logger.SetLevel(*parsedArgs.LogLevel, *parsedArgs.LogLevel)
	} else {
		logger.SetLevel(*parsedArgs.DriverLogLevel, *parsedArgs.InstanceLogLevel)
	}

	// Create an Agent object
	agent, err := newAgent(aws.BoolValue(parsedArgs.BlackholeEC2Metadata), parsedArgs.AcceptInsecureCert)
	if err != nil {
		// Failure to initialize either the docker client or the EC2 metadata
		// service client are non terminal errors as they could be transient
		return exitcodes.ExitError
	}

	if agent.getConfig().EnableRuntimeStats.Enabled() {
		defer logger.StartRuntimeStatsLogger(agent.getConfig())()
	}

	switch {
	case *parsedArgs.ECSAttributes:
		// Print agent's ecs attributes based on its environment and exit
		return agent.printECSAttributes()
	case *parsedArgs.WindowsService:
		// Enable Windows Service
		return agent.startWindowsService()
	default:
		// Start the agent
		return agent.start()
	}
}
