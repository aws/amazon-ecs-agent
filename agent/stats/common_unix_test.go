//+build !windows

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package stats

import (
	"context"
	"os"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/clientfactory"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	sdkClient "github.com/docker/docker/client"
	docker "github.com/fsouza/go-dockerclient"
)

var (
	testImageName    = "amazon/amazon-ecs-gremlin:make"
	endpoint         = utils.DefaultIfBlank(os.Getenv(ecsengine.DockerEndpointEnvVariable), ecsengine.DockerDefaultEndpoint)
	client, _        = docker.NewClient(endpoint)
	sdkclient, _     = sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint))
	clientFactory    = clientfactory.NewFactory(context.TODO(), endpoint)
	sdkClientFactory = sdkclientfactory.NewFactory(context.TODO(), endpoint)
	ctx              = context.TODO()
)
