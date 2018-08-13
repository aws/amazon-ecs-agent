# Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
# This script is used to generate windows docker images used by agent
# integration tests and functions tests and upload images to ECR.

Param(
    [string]$GIT_REPO,
    [string]$COMMIT_SHA,
    [string]$S_REPOSITORY,
    [string]$R_REPOSITORY,
    [string]$S_REGION,
    [string]$R_REGION
)

$ErrorActionPreference = 'Continue'

# Constants
$GitVersion = '2.10.1'
$GoVersion = '1.9'
$WORK_ROOT = Join-Path $ENV:TMP 'amazon-ecs-agent'
$GOPATH = Join-Path $WORK_ROOT 'go'
$BUILD_ROOT = Join-Path $GOPATH 'src\github.com\aws\amazon-ecs-agent'

if(!$(Test-Path -Path $WORK_ROOT)) {
  New-Item -Path $WORK_ROOT -Type directory
}

echo "==============================================================================="
echo "Building windows images for commit ${COMMIT_SHA}"
echo "==============================================================================="

# Install Git
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
echo 'Downloading Git...'
Invoke-WebRequest "https://github.com/git-for-windows/git/releases/download/v${GitVersion}.windows.1/Git-${GitVersion}-64-bit.exe" -OutFile "${ENV:TEMP}\git-installer.exe" -UseBasicParsing
echo 'Installing Git...'
Start-Process $ENV:TEMP\git-installer.exe -ArgumentList '/SILENT /COMPONENTS="icons,ext,ext\reg\shellhere,assoc,assoc_sh"' -NoNewWindow -Wait
$ENV:Path = ${ENV:Path} + ";${ENV:ProgramFiles}/Git/cmd"
[Environment]::SetEnvironmentVariable("Path", $ENV:Path + ";${ENV:ProgramFiles}/Git/cmd", [EnvironmentVariableTarget]::Machine)
git config --global user.name "nobody"
git config --global user.email "nobody@example.localhost"

# Install Go
echo 'Downloading Go...'
Invoke-WebRequest "https://storage.googleapis.com/golang/go${GoVersion}.windows-amd64.msi" -OutFile "${ENV:TEMP}\go-installer.msi" -UseBasicParsing
echo 'Installing Go...'
Start-Process "msiexec.exe" -ArgumentList "/quiet /i ${ENV:TEMP}\go-installer.msi" -Wait
New-Item -ItemType directory -Path "${WORK_ROOT}\go\bin"
New-Item -ItemType directory -Path "${WORK_ROOT}\go\src"
[Environment]::SetEnvironmentVariable("Path", $ENV:Path + ";C:\Go\bin;${WORK_ROOT}\go\bin", [EnvironmentVariableTarget]::Machine)
[Environment]::SetEnvironmentVariable("GOPATH", "${WORK_ROOT}\go", [EnvironmentVariableTarget]::Machine)
$ENV:Path=$ENV:Path + ";C:\Go\bin;${WORK_ROOT}\go\bin"
$ENV:GOPATH="${WORK_ROOT}\go"

echo "Checking out ${GIT_REPO} at ${COMMIT_SHA}"
git clone "${GIT_REPO}" "${BUILD_ROOT}" 2>&1 | % {$_.ToString()}
cd "${BUILD_ROOT}"
git config --add remote.origin.fetch '+refs/pull/*/head:refs/remotes/origin/pr/*'
git fetch 2>&1 | % {$_.ToString()}
git checkout "${COMMIT_SHA}" 2>&1 | % {$_.ToString()}

echo "Starting Docker..."
Start-Service -Name 'docker'

echo "Waiting for Docker to start..."
$dockerSrv = Get-Service -Name 'docker'
$stat = $dockerSrv.WaitForStatus('Running', '00:15:00')

echo "Waiting for Docker to become responsive..."
docker ps
if (${LastExitCode} -ne 0) {
  echo "Docker ps didn't go well." -logLevel "ERROR"
  echo $_.Exception.Message -logLevel "ERROR"
}

echo "-------------------------------------------------------------------------------"
echo "Generating Windows Images..."
echo "-------------------------------------------------------------------------------"

$ErrorActionPreference="Stop"

$application = Get-WmiObject Win32_Product -filter "Name='AWS Tools for Windows'"
if ($application) {
    $application.Uninstall()
}
Install-Module -Name AWSPowerShell -Force
Import-Module AWSPowerShell

echo "Pulling microsoft/windowsservercore:latest..."
docker pull microsoft/windowsservercore:latest

$ENV:ECS_WINDOWS_TEST_DIR="${BUILD_ROOT}"
$ENV:ECS_FTEST_TMP="${WORK_ROOT}\ftest_temp"
New-Item -Path "${ENV:ECS_FTEST_TMP}" -Type directory

echo "Build root: ${BUILD_ROOT}"

# Prepare dependencies
Invoke-Expression "${BUILD_ROOT}\misc\windows-iam\Setup_Iam_Images.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\volumes-test\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\image-cleanup-test-images\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\stats-windows\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\netkitten\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\windows-listen80\Setup_Listen80.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\windows-telemetry\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\windows-python\build.ps1"
Invoke-Expression "${BUILD_ROOT}\misc\container-health-windows\build.ps1"

$sAccessKey=(Get-SSMParameter -Name SAccessKey -WithDecryption $TRUE).Value
$sSecretKey=(Get-SSMParameter -Name SSecretKey -WithDecryption $TRUE).Value
$accessKey=(Get-SSMParameter -Name AccessKey -WithDecryption $TRUE).Value
$secretKey=(Get-SSMParameter -Name SecretKey -WithDecryption $TRUE).Value

# Login ECR
Set-AWSCredential -AccessKey $sAccessKey -SecretKey $sSecretKey -StoreAs default
Set-DefaultAWSRegion -Region $S_REGION
$command = Invoke-Expression "(Get-ECRLoginCommand).Command"
Invoke-Expression $command

$array_make = @('amazon-ecs-windows-telemetry-test', 'amazon-ecs-windows-python', 'amazon-ecs-containerhealthcheck', 'amazon-ecs-volumes-test', 'image-cleanup-test-image1', 'image-cleanup-test-image2', 'image-cleanup-test-image3', 'image-cleanup-test-image4', 'image-cleanup-test-image5', 'image-cleanup-test-image6', 'image-cleanup-test-image7', 'image-cleanup-test-image8', 'image-cleanup-test-image9', 'image-cleanup-test-image10', 'amazon-ecs-stats', 'amazon-ecs-netkitten')

$array_latest = @('amazon-ecs-iamrolecontainer', 'amazon-ecs-listen80')

foreach ($element in $array_make){
    $result=(GET-ECRRepository -Region "${S_REGION}"|Select-Object RepositoryName|Select-String -Pattern "windows/$element")
    if ($result -eq $null) { New-ECRRepository -Region "${S_REGION}" -RepositoryName "windows/${element}" }
    docker tag "amazon/${element}:make" "${S_REPOSITORY}/windows/${element}:make"
    docker push "${S_REPOSITORY}/windows/${element}:make"
    docker rmi "${S_REPOSITORY}/windows/${element}:make"
}

foreach ($element in $array_latest){
    $result=(GET-ECRRepository -Region "${S_REGION}"|Select-Object RepositoryName|Select-String -Pattern "windows/$element")
    if ($result -eq $null) { New-ECRRepository -Region "${S_REGION}" -RepositoryName "windows/${element}" }
    docker tag "amazon/${element}:latest" "${S_REPOSITORY}/windows/${element}:latest"
    docker push "${S_REPOSITORY}/windows/${element}:latest"
    docker rmi "${S_REPOSITORY}/windows/${element}:latest"
}

#upload to replication ecr
Set-AWSCredential -AccessKey $accessKey -SecretKey $secretKey -StoreAs default
Set-DefaultAWSRegion -Region $R_REGION
$command = Invoke-Expression "(Get-ECRLoginCommand).Command"
Invoke-Expression $command

foreach ($element in $array_make){
    $result=(GET-ECRRepository -Region "${R_REGION}"|Select-Object RepositoryName|Select-String -Pattern "windows/$element")
    if ($result -eq $null) { New-ECRRepository -Region "${R_REGION}" -RepositoryName "windows/${element}" }
    docker tag "amazon/${element}:make" "${R_REPOSITORY}/windows/${element}:make"
    docker push "${R_REPOSITORY}/windows/${element}:make"
    docker rmi "${R_REPOSITORY}/windows/${element}:make"
}

foreach ($element in $array_latest){
    $result=(GET-ECRRepository -Region "${R_REGION}"|Select-Object RepositoryName|Select-String -Pattern "windows/$element")
    if ($result -eq $null) { New-ECRRepository -Region "${R_REGION}" -RepositoryName "windows/${element}" }
    docker tag "amazon/${element}:latest" "${R_REPOSITORY}/windows/${element}:latest"
    docker push "${R_REPOSITORY}/windows/${element}:latest"
    docker rmi "${R_REPOSITORY}/windows/${element}:latest"
}

Set-AWSCredential -AccessKey $sAccessKey -SecretKey $sSecretKey -StoreAs default
Set-DefaultAWSRegion -Region $S_REGION
exit 0