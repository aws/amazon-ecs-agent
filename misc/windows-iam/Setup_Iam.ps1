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

$ErrorActionPreference = 'Stop'

# Download devcon binary
$devconZipUri = "https://s3.amazonaws.com/amazon-ecs-devcon/devcon-3a3f98688359aa1b905147831f910218.zip"
$devconZipMD5Uri = "$devconZipUri.md5"
$devconZipFile = "${PSScriptRoot}\devcon.zip"
$devconMD5File = "${PSScriptRoot}\devcon.zip.md5"
Invoke-RestMethod -OutFile $devconZipFile -Uri $devconZipUri
Invoke-RestMethod -OutFile $devconMD5File -Uri $devconZipMD5Uri

$expectedMD5 = (Get-Content $devconMD5File)
$md5 = New-Object -TypeName System.Security.Cryptography.MD5CryptoServiceProvider
$actualMD5 = [System.BitConverter]::ToString($md5.ComputeHash([System.IO.File]::ReadAllBytes($devconZipFile))).replace('-', '')

if($expectedMD5 -ne $actualMD5) {
    echo "devcon download doesn't match hash."
    echo "Expected: $expectedMD5 - Got: $actualMD5"
    exit 1
}

Expand-Archive -Path $devconZipFile -DestinationPath ${PSScriptRoot} -Force

# Set up host
$wd=(pwd).Path
try {
    cd "${PSScriptRoot}"
    Invoke-Expression "..\windows-deploy\loopback.ps1"
    Invoke-Expression "..\windows-deploy\hostsetup.ps1"
    Invoke-Expression "docker build -t amazon/amazon-ecs-credential-proxy --file ..\windows-deploy\credentialproxy.dockerfile ..\windows-deploy"
} finally {
    cd $wd
}

# Create amazon/amazon-ecs-iamrolecontainer for tests
$buildscript = @"
mkdir C:\IAM
cp C:\ecs\ec2.go C:\IAM
go get -u  github.com/aws/aws-sdk-go
go get -u  github.com/aws/aws-sdk-go/aws
go build -o C:\IAM\ec2.exe C:\IAM\ec2.go
cp C:\IAM\ec2.exe C:\ecs
"@

docker run `
  --volume ${PSScriptRoot}:C:\ecs `
  golang:1.7-windowsservercore `
  powershell ${buildscript}

Invoke-Expression "docker build -t amazon/amazon-ecs-iamrolecontainer --file ${PSScriptRoot}\iamroles.dockerfile ${PSScriptRoot}"
