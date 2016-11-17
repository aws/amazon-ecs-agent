# Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

Invoke-Expression "${PSScriptRoot}\..\misc\windows-iam\Setup_Iam.ps1"
Invoke-Expression "${PSScriptRoot}\..\misc\windows-listen80\Setup_Listen80.ps1"

#First stop/remove any existing credential proxy containers
$credentialProxy = "ecs-cred-proxy"
docker inspect ${credentialProxy}
if (${LastExitCode} -eq 0) {
    try {
        docker stop ${credentialProxy}
        docker rm ${credentialProxy}
    } catch {
        exit 1
    }
}

# Set up the proxy
Invoke-Expression "${PSScriptRoot}\..\misc\windows-deploy\setupcredentialproxy.ps1"

# Run the tests
$cwd = (pwd).Path
try {
  cd "${PSScriptRoot}"
  go test -tags functional -timeout=30m -v ../agent/functional_tests/tests
  go test -tags functional -timeout=30m -v ../agent/functional_tests/tests/generated/simpletests_windows
  $testsExitCode = $LastExitCode
} finally {
  cd "$cwd"
  docker stop ecs-cred-proxy
}

exit $testsExitCode
