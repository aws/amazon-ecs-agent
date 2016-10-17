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

Write-Host "Configuring proxy..."
$gateway = (Get-WMIObject -Class Win32_IP4RouteTable | Where { $_.Destination -eq '0.0.0.0' -and $_.Mask -eq '0.0.0.0' } | Sort-Object Metric1 | Select NextHop).NextHop
Write-Host "Gateway is "
Write-Output $gateway
netsh interface portproxy add v4tov4 listenaddress=0.0.0.0 listenport=51679 connectaddress=$gateway connectport=51679 protocol=tcp
Write-Host "Configured netsh portproxy"

Write-Host "Sleeping"
While ($true) {
  Start-Sleep -s 600
}
