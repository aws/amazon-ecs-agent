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
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

type TelemetrySessionParams struct {
	ContainerInstanceArn string
	CredentialProvider   *credentials.Credentials
	Cfg                  *config.Config
	DockerClient         engine.DockerClient
	AcceptInvalidCert    bool
	EcsClient            api.ECSClient
	TaskEngine           engine.TaskEngine
}

func (params *TelemetrySessionParams) isTelemetryDisabled() (bool, error) {
	if params.Cfg != nil {
		return params.Cfg.DisableMetrics, nil
	}
	return false, fmt.Errorf("Config is not initialized in session params")
}
