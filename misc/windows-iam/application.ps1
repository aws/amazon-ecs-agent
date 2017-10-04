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

$CREDENTIAL_ADDRESS = "169.254.170.2"
$EC2_METADATA_ADDRESS = "169.254.169.254"
[string]$gateway = (Get-WMIObject -Class Win32_IP4RouteTable | Where { $_.Destination -eq '0.0.0.0' -and $_.Mask -eq '0.0.0.0' } | Sort-Object Metric1 | Select NextHop).NextHop
[int]$ifIndex = (Get-NetAdapter -InterfaceDescription "Hyper-V Virtual Ethernet*" | Sort-Object | Select -First 1).ifIndex
New-NetRoute -DestinationPrefix "$($CREDENTIAL_ADDRESS)/32" -InterfaceIndex $ifindex -NextHop $gateway
New-NetRoute -DestinationPrefix "$($EC2_METADATA_ADDRESS)/32" -InterfaceIndex $ifindex -NextHop $gateway
