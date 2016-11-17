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

.\devcon /r remove =net "@ROOT\NET\*"
.\devcon -r install C:\Windows\INF\netloop.inf *MSLOOP
[int]$retryattempts = 5

$networkAdapters = Get-CimInstance -ClassName Win32_NetworkAdapter -Filter "AdapterTypeId='0' AND NetEnabled='True' AND Name LIKE '%Loopback Adapter%'" | Sort-Object -Property "Index"
while(-not $networkAdapters -or $networkAdapters.Length -eq 0) {
    if ($retryattempts -eq 0) {
        throw New-Object System.Exception("Failed to find the loopback network interface. Task IAM roles would not work")
        exit 2
    } else {
        $networkAdapters = Get-CimInstance -ClassName Win32_NetworkAdapter -Filter "AdapterTypeId='0' AND NetEnabled='True' AND Name LIKE '%Loopback Adapter%'" | Sort-Object -Property "Index"
        $retryattempts--
   }
}
$loopbackIndex = $networkAdapters[0].InterfaceIndex

$loopbackName = (Get-NetAdapter -InterfaceIndex $loopbackIndex | Sort-Object | Select Name).Name

netsh int ip set address name=$loopbackName static 169.254.170.2 255.255.255.0
