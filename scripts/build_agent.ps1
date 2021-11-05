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
#
# Standalone Amazon ECS Container Agent for Windows may be built by using this script

$cwd = (pwd).Path

$pauseImageName = "amazon/amazon-ecs-pause"
$pauseImageTag = "0.1.0"

$build_exe = "../amazon-ecs-agent.exe"
$ldflag = "-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerTag=$pauseImageTag -X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerImageName=$pauseImageName"

cd ../agent/
go build -ldflags "$ldflag" -o $build_exe .
cd "$cwd"
