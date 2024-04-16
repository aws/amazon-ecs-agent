//go:build !windows
// +build !windows

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

package sdkclientfactory

import (
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient"
)

// GetClient on linux will simply return the cached client from the map
func (f *factory) GetClient(version dockerclient.DockerVersion) (sdkclient.Client, error) {
	return f.getClient(version)
}

// getAgentSupportedDockerVersions returns a list of agent-supported Docker versions for linux
func getAgentSupportedDockerVersions() []dockerclient.DockerVersion {
	return dockerclient.GetKnownAPIVersions()
}

// getDefaultVersion will return the default Docker API version for linux
func GetDefaultVersion() dockerclient.DockerVersion {
	return dockerclient.MinDockerAPIVersion
}
