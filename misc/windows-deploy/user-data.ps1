<powershell>
## The string 'windows' should be replaced with your cluster name

# Set agent env variables for the Machine context (durable)
[Environment]::SetEnvironmentVariable("ECS_CLUSTER", "windows", "Machine")
[Environment]::SetEnvironmentVariable("ECS_ENABLE_TASK_IAM_ROLE", "false", "Machine")
$agentVersion = 'v1.15.2'
$agentZipUri = "https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-windows-$agentVersion.zip"
$agentZipSha256Uri = "$agentZipUri.sha256"


### --- Nothing user configurable after this point ---
$ecsExeDir = "$env:ProgramFiles\Amazon\ECS"
$zipFile = "$env:TEMP\ecs-agent.zip"
$sha256File = "$env:TEMP\ecs-agent.zip.sha256"

### Get the files from S3
Invoke-RestMethod -OutFile $zipFile -Uri $agentZipUri
Invoke-RestMethod -OutFile $sha256File -Uri $agentZipSha256Uri

## SHA256 Checksum
$expectedSHA256 = (Get-Content $sha256File)
$sha256 = New-Object -TypeName System.Security.Cryptography.SHA256CryptoServiceProvider
$actualSHA256 = [System.BitConverter]::ToString($sha256.ComputeHash([System.IO.File]::ReadAllBytes($zipFile))).replace('-', '')

if($expectedSHA256 -ne $actualSHA256) {
    echo "Download doesn't match hash."
    echo "Expected: $expectedSHA256 - Got: $actualSHA256"
    exit 1
}

## Put the executables in the executable directory.
Expand-Archive -Path $zipFile -DestinationPath $ecsExeDir -Force

## Start the agent script in the background.
$jobname = "ECS-Agent-Init"
$script =  "cd '$ecsExeDir'; .\amazon-ecs-agent.ps1"
$repeat = (New-TimeSpan -Minutes 1)

$jobpath = $env:LOCALAPPDATA + "\Microsoft\Windows\PowerShell\ScheduledJobs\$jobname\ScheduledJobDefinition.xml"
if($(Test-Path -Path $jobpath)) {
  echo "Job definition already present"
  exit 0

}

$scriptblock = [scriptblock]::Create("$script")
$trigger = New-JobTrigger -At (Get-Date).Date -RepeatIndefinitely -RepetitionInterval $repeat -Once
$options = New-ScheduledJobOption -RunElevated -ContinueIfGoingOnBattery -StartIfOnBattery
Register-ScheduledJob -Name $jobname -ScriptBlock $scriptblock -Trigger $trigger -ScheduledJobOption $options -RunNow
Add-JobTrigger -Name $jobname -Trigger (New-JobTrigger -AtStartup -RandomDelay 00:1:00)
</powershell>
<persist>true</persist>
