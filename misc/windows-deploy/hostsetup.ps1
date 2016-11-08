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

$networkAdapters = Get-CimInstance -ClassName Win32_NetworkAdapter -Filter "AdapterTypeId='0' AND NetEnabled='True' AND NOT Name LIKE '%Hyper-V Virtual%' AND NOT Name LIKE '%Loopback Adapter%' AND NOT Name LIKE '%TAP-Windows Adapter%'" | Sort-Object -Property "Index"
if(-not $networkAdapters -or $networkAdapters.Length -eq 0)
{
    throw New-Object System.Exception("Failed to find the primary network interface")
}
$ifIndex = $networkAdapters[0].InterfaceIndex
$networkAdapterConfig = Get-CimInstance -ClassName Win32_NetworkAdapterConfiguration -Filter "InterfaceIndex='$ifIndex'" | select IPConnectionMetric, DefaultIPGateway
$defaultGateway = $networkAdapterConfig.DefaultIPGateway[$networkAdapterConfig.DefaultIPGateway.Length - 1]

$containerGateway = (Get-NetNat).InternalIPInterfaceAddressPrefix | %{ $_ -replace '/.*$', '' }

netsh interface portproxy add v4tov4 listenaddress=$containerGateway listenport=51679 connectaddress=127.0.0.1 connectport=51679 protocol=tcp

route -p delete 169.254.170.2/32
New-NetRoute -DestinationPrefix 169.254.170.2/32 -InterfaceIndex $ifIndex -NextHop $defaultGateway
