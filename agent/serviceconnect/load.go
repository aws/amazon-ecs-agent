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

package serviceconnect

import (
	utilsloader "github.com/aws/amazon-ecs-agent/agent/utils/loader"
)

var (
	defaultAgentContainerImageName = "appnet_agent"
	defaultAgentContainerTag       = "service_connect.v1"
)

// Loader defines an interface for loading the appnetAgent container image. This is mostly
// to facilitate mocking and testing of the LoadImage method
type Loader interface {
	utilsloader.Loader

	GetLoadedImageName() (string, error)
}

type agentLoader struct {
	AgentContainerImageName   string
	AgentContainerTag         string
	AgentContainerTarballPath string
}

// New creates a new AppNet Agent image loader
func New() Loader {
	return &agentLoader{
		AgentContainerImageName:   defaultAgentContainerImageName,
		AgentContainerTag:         defaultAgentContainerTag,
		AgentContainerTarballPath: defaultAgentContainerTarballPath,
	}
}
