# Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved
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

$oldPref = $ErrorActionPreference
$ErrorActionPreference = 'Stop'

Invoke-Expression "Import-Module ECSTools; Initialize-ECSHost"

# Create amazon/amazon-ecs-v3-task-endpoint-validator-windows for tests
Invoke-Expression "go get github.com/docker/docker/api/types github.com/pkg/errors"
Invoke-Expression "go build -o ${PSScriptRoot}\v3-task-endpoint-validator-windows.exe ${PSScriptRoot}\v3-task-endpoint-validator-windows.go" 
Invoke-Expression "docker build -t amazon/amazon-ecs-v3-task-endpoint-validator-windows --file ${PSScriptRoot}\v3-task-endpoint-validator-windows.dockerfile ${PSScriptRoot}"
$ErrorActionPreference = $oldPref
