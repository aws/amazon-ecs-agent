//go:build windows
// +build windows

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

// minDockerAPIVersion is the min Docker API version supported by agent
const minDockerAPIVersion = dockerclient.Version_1_24

// GetClient will replace some versions of Docker on Windows. We need this because
// agent assumes that it can always call older versions of the docker API.
func (f *factory) GetClient(version dockerclient.DockerVersion) (sdkclient.Client, error) {
	for _, v := range getWindowsReplaceableVersions() {
		if v == version {
			version = minDockerAPIVersion
			break
		}
	}
	return f.getClient(version)
}

// getWindowsReplaceableVersions returns the set of versions that agent will report
// as Docker 1.24
func getWindowsReplaceableVersions() []dockerclient.DockerVersion {
	return []dockerclient.DockerVersion{
		dockerclient.Version_1_17,
		dockerclient.Version_1_18,
		dockerclient.Version_1_19,
		dockerclient.Version_1_20,
		dockerclient.Version_1_21,
		dockerclient.Version_1_22,
		dockerclient.Version_1_23,
	}
}

// getWindowsSupportedVersions returns the set of remote api versions that are
// supported by agent in windows
func getWindowsSupportedVersions() []dockerclient.DockerVersion {
	return []dockerclient.DockerVersion{
		dockerclient.Version_1_24,
		dockerclient.Version_1_25,
		dockerclient.Version_1_26,
		dockerclient.Version_1_27,
		dockerclient.Version_1_28,
		dockerclient.Version_1_29,
		dockerclient.Version_1_30,
	}
}

// getAgentSupportedDockerVersions for Windows should return all of the replaceable versions plus supported versions
func getAgentSupportedDockerVersions() []dockerclient.DockerVersion {
	return append(getWindowsReplaceableVersions(), getWindowsSupportedVersions()...)
}

// getDefaultVersion returns agent's default version of the Docker API
func GetDefaultVersion() dockerclient.DockerVersion {
	return minDockerAPIVersion
}
