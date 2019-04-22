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

$oldActionPref = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

# This setup script is required in order to enable task IAM roles on windows instances.

# 169.254.170.2:51679 is the IP address used for task IAM roles.
$credentialAddress = "169.254.170.2"
$credentialPort = "51679"
$loopbackAddress = "127.0.0.1"

$adapter = (Get-NetAdapter -Name "*APIPA*")
if(!($adapter)) {
	# APIPA nat adapter controls IP range 169.254.x.x on windows.
	Add-VMNetworkAdapter -VMNetworkAdapterName "APIPA" -SwitchName nat -ManagementOS
}

$ifIndex = (Get-NetAdapter -Name "*APIPA*" | Sort-Object | Select ifIndex).ifIndex

$dockerSubnet = (docker network inspect nat | ConvertFrom-Json).IPAM.Config.Subnet

# This address will only exist on systems that have already set up the routes.
$ip = (Get-NetRoute -InterfaceIndex $ifIndex -DestinationPrefix $dockerSubnet)
if(!($ip)) {

	$IPAddrParams = @{
		InterfaceIndex = $ifIndex;
		IPAddress = $credentialAddress;
		PrefixLength = 32;
	}

	# This command tells the APIPA interface that this IP exists.
	New-NetIPAddress @IPAddrParams

	[int]$delay = 1
	[int]$maxTries = 10
	[bool]$available = $false
	while (-not $available -and $maxTries -ge 0) {
		try {
			$IPAddr = Get-NetIPAddress @IPAddrParams -ErrorAction:Ignore
			if ($IPAddr) {
				if ($($IPAddr.AddressState) -eq "Preferred") {
					$available = $true
					break;
				} else {
					Start-Sleep -Seconds $delay
					$maxTries--
				}
			}
		} catch {
			# Prevent race condition where the adapter has multiple addresses before our new ip is assigned
			# Note: Allowing this race condition can cause the netsh interface portproxy setup to result
			#       in the network adapter in an unrecoverable bad state where the proxy is unable to
			#       listen on port 80 because the port is in use by a non-functional proxy rule.
			Start-Sleep -Seconds $delay
			$maxTries--
		}
	}

	# Enable the default docker IP range to be routable by the APIPA interface.
	New-NetRoute -DestinationPrefix $dockerSubnet -ifIndex $ifindex

	# Exposes credential port for local windows firewall
	New-NetFirewallRule -DisplayName "Allow Inbound Port $credentialPort" -Direction Inbound -LocalPort $credentialPort -Protocol TCP -Action Allow

	# This forwards traffic from port 80 and listens on the IAM role IP address.
	# 'portproxy' doesn't have a powershell module equivalent, but we could move if it becomes available.
	netsh interface portproxy add v4tov4 listenaddress=$credentialAddress listenport=80 connectaddress=$loopbackAddress connectport=$credentialPort
}

$ErrorActionPreference=$oldActionPref
