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

Invoke-Expression ${PSScriptRoot}\..\windows-deploy\hostsetup.ps1

# Create amazon/amazon-ecs-container-metadata-file-validator-windows for tests
$buildscript = @"
mkdir C:\md
cp C:\ecs\container-metadata-file-validator-windows.go C:\md
go build -o C:\md\container-metadata-file-validator-windows.exe C:\md\container-metadata-file-validator-windows.go
cp C:\md\container-metadata-file-validator-windows.exe C:\ecs
"@

$buildimage="golang:1.7-nanoserver"
docker pull $buildimage

docker run `
  --volume ${PSScriptRoot}:C:\ecs `
  $buildimage `
  powershell ${buildscript}

Invoke-Expression "docker build -t amazon/amazon-ecs-container-metadata-file-validator-windows --file ${PSScriptRoot}\container-metadata-file-validator-windows.dockerfile ${PSScriptRoot}"
$ErrorActionPreference = $oldPref
