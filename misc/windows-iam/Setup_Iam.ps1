# Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved
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

Invoke-Expression "go get -u  github.com/aws/aws-sdk-go"
Invoke-Expression "go get -u  github.com/aws/aws-sdk-go/aws"
Invoke-Expression "go build -o ${PSScriptRoot}\ec2.exe ${PSScriptRoot}\ec2.go"

Invoke-Expression "docker build -t amazon/amazon-ecs-iamrolecontainer --file ${PSScriptRoot}\iamroles.dockerfile ${PSScriptRoot}"
$ErrorActionPreference = $oldPref
