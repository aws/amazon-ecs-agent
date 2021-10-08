# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

go env -w GO111MODULE=off
go build -tags integration -o ${PSScriptRoot}\sleep.exe ${PSScriptRoot}\sleep\

# create the container image used to run execcmd integ tests
docker build -t "amazon/amazon-ecs-exec-command-agent-windows-test:make" -f "${PSScriptRoot}/windows.dockerfile" ${PSScriptRoot}

$SIMULATED_ECS_AGENT_DEPS_BIN_DIR="C:\Program Files\Amazon\ECS\managed-agents\execute-command\bin\1.0.0.0"
Remove-Item -Path $SIMULATED_ECS_AGENT_DEPS_BIN_DIR -Recurse -Force -ErrorAction SilentlyContinue
New-Item -Path $SIMULATED_ECS_AGENT_DEPS_BIN_DIR -ItemType Directory -Force
Move-Item -Path ${PSScriptRoot}\sleep.exe -Destination $SIMULATED_ECS_AGENT_DEPS_BIN_DIR\amazon-ssm-agent.exe
New-Item -Path $SIMULATED_ECS_AGENT_DEPS_BIN_DIR\ssm-agent-worker.exe -ItemType File -Force
New-Item -Path $SIMULATED_ECS_AGENT_DEPS_BIN_DIR\ssm-session-worker.exe -ItemType File -Force

# Dont want to destroy local development environments plugin folder
$SIMULATED_SSM_PLUGINS_DIR="C:\Program Files\Amazon\SSM\Plugins"
if(!(Test-Path -path $SIMULATED_SSM_PLUGINS_DIR))
{
    New-Item -Path $SIMULATED_SSM_PLUGINS_DIR -ItemType directory -Force
}

if(!(Test-Path -path $SIMULATED_SSM_PLUGINS_DIR\SessionManagerShell))
{
    New-Item -Path $SIMULATED_SSM_PLUGINS_DIR\SessionManagerShell -ItemType directory -Force
}

if(!(Test-Path -path $SIMULATED_SSM_PLUGINS_DIR\awsCloudWatch))
{
    New-Item -Path $SIMULATED_SSM_PLUGINS_DIR\awsCloudWatch -ItemType directory -Force
}

if(!(Test-Path -path $SIMULATED_SSM_PLUGINS_DIR\awsDomainJoin))
{
    New-Item -Path $SIMULATED_SSM_PLUGINS_DIR\awsDomainJoin -ItemType directory -Force
}

# Set the Go Modules to auto once we have built the binaries.
go env -w GO111MODULE=auto