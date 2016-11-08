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


$ecsRootDir = "C:\ProgramData\Amazon ECS\"

function LogMsg([string]$message = "Logging no message", $logLevel = "INFO") {
     $logdir = $ecsRootDir + "log\win-agent-init.log"
     Add-Content $logdir "$(Get-Date)    [$logLevel] $message"
}

$env:ECS_DATADIR = $ecsRootDir + "data"
$env:ECS_LOGFILE = $ecsRootDir + "log\ecs-agent.log"
$env:ECS_AUDIT_LOGFILE = $ecsRootDir + "log\audit.log"
$env:ECS_DISABLE_METRICS = "true"
$env:ECS_LOGLEVEL = "debug"
$env:ECS_AVAILABLE_LOGGING_DRIVERS = "[`"json-file`",`"awslogs`"]"


try {
    $datadir = $ecsRootDir+"data"
    if(!$(Test-Path -Path  $datadir)) {
        mkdir $datadir
    }
    $logdir = $ecsRootDir+"log"
    if(!$(Test-Path -Path $logdir)) {
        mkdir $logdir
    }
} catch  {
    echo $_.Exception.Message
    exit 1
}

if(!$(Test-Path env:ECS_CLUSTER)) {
    $env:ECS_CLUSTER = "windows"
}

LogMsg "Checking if docker is running."

try {
    $dockerSrv = Get-Service -Name 'docker'
    $stat = $dockerSrv.WaitForStatus('Running', '00:15:00')
    LogMsg "Docker is running. Running 'docker ps'."


    LogMsg "First stop/remove any existing credential proxy containers"

    $credentialProxy = "ecs-cred-proxy"
    docker inspect ${credentialProxy}
    if (${LastExitCode} -eq 0) {
        try {
            LogMsg "Stopping/Removing credential proxy."
            docker stop ${credentialProxy}
            docker rm ${credentialProxy}
        } catch {
            LogMsg -message "Stopping/Removing Credential Proxy container failed" -logLevel "INFO"
            LogMsg -message "IAM roles may not work. Try manually stopping/removing the container before running this script again." -logLevel "INFO"
            exit 1
        }
    }


    if([System.Environment]::GetEnvironmentVariable("ECS_ENABLE_TASK_IAM_ROLE", "Machine") -eq "true") {
        LogMsg "IAM roles environment variable is set."

        .\loopback.ps1
        .\hostsetup.ps1

        try {
            docker build -t amazon/amazon-ecs-credential-proxy --file .\credentialproxy.dockerfile . | out-null
            docker run --name ecs-cred-proxy -d -p 80:51679 amazon/amazon-ecs-credential-proxy | out-null
        } catch {
            LogMsg -message "Running Credential Proxy container failed" -logLevel "INFO"
            LogMsg -message $_.Exception.Message -logLevel "ERROR"
            exit 2
        }
    }
    LogMsg "Docker is good to go!"
    LogMsg "Starting agent... here we go!"
    try {
        .\agent.exe
    } catch {
        LogMsg "Could not start agent.exe."
        LogMsg -message $_.Exception.Message -logLevel "INFO"
        exit 2
    }
} catch [System.ServiceProcess.TimeoutException] {
    LogMsg -message "Docker not started before timeout." -logLevel "ERROR"
    exit 3
}
