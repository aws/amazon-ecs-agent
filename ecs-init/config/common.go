// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.
package config

const (
	AgentImageName        = "amazon/amazon-ecs-agent:latest"
	AgentContainerName    = "ecs-agent"
	AgentLogFile          = "ecs-agent.log"
	AgentRemoteTarball    = "https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-latest.tar"
	AgentRemoteTarballMD5 = AgentRemoteTarball + ".md5"
)

func AgentConfigDirectory() string {
	return directoryPrefix + "/etc/ecs"
}

func AgentConfigFile() string {
	return AgentConfigDirectory() + "/ecs.config"
}

func AgentJSONConfigFile() string {
	return AgentConfigDirectory() + "/ecs.config.json"
}

func LogDirectory() string {
	return directoryPrefix + "/var/log/ecs"
}

func initLogFile() string {
	return LogDirectory() + "/ecs-init.log"
}

func AgentDataDirectory() string {
	return directoryPrefix + "/var/lib/ecs/data"
}

func CacheDirectory() string {
	return directoryPrefix + "/var/cache/ecs"
}

func AgentTarball() string {
	return CacheDirectory() + "/ecs-agent.tar"
}
