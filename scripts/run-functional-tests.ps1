# Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
  [string]$Platform="windows2016"
)

if ($Platform -like "windows2016") {
  $BaseImageName="mcr.microsoft.com/windows/servercore:ltsc2016"
} elseif ($Platform -like "windows2019")  {
  $BaseImageName="mcr.microsoft.com/windows/servercore:ltsc2019"
} else {
  echo "Invalid platform parameter"
  exit 1
}

# Prepared base image
$dockerImages = Invoke-Expression "docker images"
if (-Not ($dockerImages -like "*$BaseImageName*")) {
    Invoke-Expression "docker pull $BaseImageName"
}
Invoke-Expression "docker tag $BaseImageName amazon-ecs-ftest-windows-base:make"

Invoke-Expression "${PSScriptRoot}\..\misc\windows-iam\Setup_Iam.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\windows-listen80\Setup_Listen80.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\windows-telemetry\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\container-health-windows\build.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\v3-task-endpoint-validator-windows\setup-v3-task-endpoint-validator.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\container-metadata-file-validator-windows\setup-container-metadata-file-validator.ps1"

# Run the tests
$cwd = (pwd).Path
try {
  cd "${PSScriptRoot}"
  go test -tags functional -timeout=60m -v ../agent/functional_tests/tests
  $handwrittenExitCode = $LastExitCode
  echo "Handwritten functional tests exited with ${handwrittenExitCode}"
  go test -tags functional -timeout=30m -v ../agent/functional_tests/tests/generated/simpletests_windows
  $simpletestExitCode = $LastExitCode
  echo "Simple functional tests exited with ${simpletestExitCode}"
} finally {
  cd "$cwd"
}
if (${handwrittenExitCode} -ne 0) {
  exit $handwrittenExitCode
}
exit $simpletestExitCode
