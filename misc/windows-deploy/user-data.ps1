<powershell>
## The string 'windows' shoud be replaced with 
## template variable for cluster name.

# Set agent env variables for the Machine context (durrable)
[Environment]::SetEnvironmentVariable("ECS_CLUSTER", "windows", "Machine")
[Environment]::SetEnvironmentVariable("ECS_ENABLE_TASK_IAM_ROLE", "false", "Machine")
$agentVersion = '1.14.0-1.windows.1'
$agentZipHash = '7748b1d3c73d5211e150db746cdec5bf'
$agentZipUri = "https://s3-us-west-2.amazonaws.com/windows-agent/ecs-agent-windows-$agentVersion.zip"


### --- Nothing user configureable after this point --- 
$ecsExeDir = "$env:ProgramFiles\Amazon\ECS"
$zipFile = "$env:TEMP\ecs-agent.zip"

### Get the files from S3
Invoke-RestMethod -OutFile $zipFile -Uri $agentZipUri

## MD5 Checksum
$md5 = New-Object -TypeName System.Security.Cryptography.MD5CryptoServiceProvider
$hash = [System.BitConverter]::ToString($md5.ComputeHash([System.IO.File]::ReadAllBytes($zipFile))).replace('-', '')

if($agentZipHash -ne $hash) {
    echo "Download doesn't match hash."
    echo "Expected: $agentZipHash - Got: $hash"
    exit 1
}

## Put the executables in the executable directory.
Expand-Archive -Path $zipFile -DestinationPath $ecsExeDir -Force

## Start the agent script in the background.

# The description of the task
$jobname = "ECS-Agent-Init"
$script =  "cd '$ecsExeDir'; .\amazon-ecs-agent.ps1"
$repeat = (New-TimeSpan -Minutes 1)

try {
    Unregister-ScheduledJob -Name $jobname | out-null
}
catch { #noop }

$scriptblock = [scriptblock]::Create("$script")
$trigger = New-JobTrigger -At (Get-Date).Date -RepeatIndefinitely -RepetitionInterval $repeat -Once
$options = New-ScheduledJobOption -RunElevated -ContinueIfGoingOnBattery -StartIfOnBattery
Register-ScheduledJob -Name $jobname -ScriptBlock $scriptblock -Trigger $trigger -ScheduledJobOption $options -RunNow
</powershell>
<persist>true</persist>
