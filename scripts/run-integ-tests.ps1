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

Param (
  [string]
  [ValidateSet("windows2016","windows2019","windows20H2", "windows2022")]
  $Platform="windows2016"
)

$ErrorActionPreference = 'Stop'

if ($Platform -like "windows2016") {
  $BaseImageName="mcr.microsoft.com/windows/servercore:ltsc2016"
} elseif ($Platform -like "windows2019")  {
  $BaseImageName="mcr.microsoft.com/windows/servercore:ltsc2019"
} elseif ($Platform -like "windows20h2")  {
  $BaseImageName="mcr.microsoft.com/windows/servercore:20H2"
} elseif ($Platform -like "windows2022")  {
  $BaseImageName="mcr.microsoft.com/windows/servercore:ltsc2022"
} else {
  echo "Invalid platform parameter"
  exit 1
}

# Prepare windows base image
$dockerImages = Invoke-Expression "docker images"
if (-Not ($dockerImages -like "*$BaseImageName*")) {
  Invoke-Expression "docker pull $BaseImageName"
}
Invoke-Expression "docker tag $BaseImageName amazon-ecs-ftest-windows-base:make"

# Ensure that "C:/Program Files/Amazon/ECS" is empty before preparing dependencies.
$ProgramFiles="C:\Program Files\Amazon\ECS"
Remove-Item -Path "$ProgramFiles\*" -Recurse -Force -ErrorAction:SilentlyContinue

# Prepare dependencies
Invoke-Expression "${PSScriptRoot}\..\misc\volumes-test\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\image-cleanup-test-images\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\stats-windows\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\container-health-windows\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\netkitten\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\exec-command-agent-test\build.ps1"

# Run the tests
$cwd = (pwd).Path
try {
  $env:GO111MODULE = 'auto'; $env:ECS_LOGLEVEL = 'debug'; go test -race -tags integration -timeout=40m -v ./agent/... ./ecs-agent/...
  $testsExitCode = $LastExitCode
} finally {
  cd "$cwd"
}

exit $testsExitCode
