//go:build linux || windows
// +build linux windows

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

package pause

import (
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
)

// This method is used to inspect the presence of the pause image. If the image has not been loaded then we return false.
func (*pauseLoader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return loader.IsImageLoaded(config.DefaultPauseContainerImageName+":"+config.DefaultPauseContainerTag, dockerClient)
}
