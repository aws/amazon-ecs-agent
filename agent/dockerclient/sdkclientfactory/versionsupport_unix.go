// +build !windows
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

package sdkclientfactory

import (
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient"
)

const (
	// minDockerAPIVersion is the min Docker API version supported by agent
	minDockerAPIVersion = dockerclient.Version_1_17
)

// GetClient on linux will simply return the cached client from the map
func (f *factory) GetClient(version dockerclient.DockerVersion) (sdkclient.Client, error) {
	return f.getClient(version)
}

// getAgentVersions returns a list of supported agent-supported versions for linux
func getAgentVersions() []dockerclient.DockerVersion {
	return []dockerclient.DockerVersion{
		dockerclient.Version_1_17,
		dockerclient.Version_1_18,
		dockerclient.Version_1_19,
		dockerclient.Version_1_20,
		dockerclient.Version_1_21,
		dockerclient.Version_1_22,
		dockerclient.Version_1_23,
		dockerclient.Version_1_24,
		dockerclient.Version_1_25,
		dockerclient.Version_1_26,
		dockerclient.Version_1_27,
		dockerclient.Version_1_28,
		dockerclient.Version_1_29,
		dockerclient.Version_1_30,
		dockerclient.Version_1_31,
		dockerclient.Version_1_32,
	}
}

// getDefaultVersion will return the default Docker API version for linux
func GetDefaultVersion() dockerclient.DockerVersion {
	return dockerclient.Version_1_21
}
