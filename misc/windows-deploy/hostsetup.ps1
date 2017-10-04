# Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved
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

# This setup script is required in order to enable task IAM roles on windows instances.
$CREDENTIAL_ADDRESS = "169.254.170.2"
$CREDENTIAL_LISTEN_PORT = "80"
$CREDENTIAL_CONNECT_PORT = "51679"

$oldActionPref = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

LogMsg "Checking VMNetwork adapter"
$VMNetParams = @{
    SwitchName = 'nat';
    ManagementOS = 1;
    Name = "*APIPA*";
}
if(!(Get-VMNetworkAdapter @VMNetParams -ErrorAction:Ignore | Select -First 1)) {
    # APIPA nat adapter controls IP range 169.254.x.x on windows.
    Add-VMNetworkAdapter -VMNetworkAdapterName "APIPA" -SwitchName nat -ManagementOS
}
[int]$delay = 1
[int]$maxTries = 10
[bool]$available = $false
While ($available -eq $false -and $maxTries -ge 0) {
    try {
        $vmNetAdapter = Get-VMNetworkAdapter @VMNetParams -ErrorAction:Ignore | Select -First 1
        if ($vmNetAdapter) {
            $available = $true
            break;
        }
    } catch {
        # VM Net Adapter not available yet
    } finally {
        LogMsg "VMNetwork adapter not found. Retrying after sleeping $($delay)sec"
        sleep $delay
        $maxTries--
    }
}
if ($available -eq $false) {
    [string]$msg = "There was a problem getting the VMNetwork adapter"
    LogMsg "ERROR: $msg"
    throw $msg
    return
} else {
    [string]$vmNetAdapterMac = $vmNetAdapter.MacAddress
    LogMsg "VMNetwork adapter found with mac: $($vmNetAdapterMac)"
}

LogMsg "Checking for network adatper with mac: $($vmNetAdapterMac)"
[int]$delay = 2
[int]$maxTries = 30
[bool]$available = $false
While (-not $available -and $maxTries -ge 0) {
    try {
        $netAdapter = Get-NetAdapter | Where {$_.MacAddress.Replace('-','') -eq $vmNetAdapterMac}
        if ($netAdapter) {
            $available = $true
            LogMsg "Network adapter found."
            Break;
        }
    } catch {
        # Net Adapter not available yet
    } finally {
        LogMsg "Network adapter not found. Retrying after sleeping $($delay)sec"
        sleep $delay
        $maxTries--
    }
}
if ($available -eq $false) {
    [string]$msg = "There was a problem getting the local network adapter adapter associated with mac: $($vmNetAdapterMac)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
} else {
    [int]$InterfaceIndex = $netAdapter.InterfaceIndex
    LogMsg "Network adapter found with mac $($vmNetAdapterMac) on interface $($InterfaceIndex)"
}

LogMsg "Getting subnet info from docker..."
try {
    $dockerCmd = "docker network inspect nat"
    $dockerNatConfigOutput = Invoke-Expression -Command $dockerCmd
    $dockerNatConfig = $dockerNatConfigOutput | ConvertFrom-Json
    $dockerSubnet = $dockerNatConfig.IPAM.Config.Subnet
    LogMsg "Docker subnet found: $($dockerSubnet)"
} catch {
    [string]$msg = "There was a problem getting the docker output. Command: $($dockerCmd), CommandOutput: $($dockerNatConfigOutput), Subnet: $($dockerSubnet), Exception: $($_.Exception.Message)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
}

LogMsg "Getting net ip address"
$IPAddrParams = @{
    InterfaceIndex = $InterfaceIndex;
    IPAddress = $CREDENTIAL_ADDRESS;
    PrefixLength = 32;
}
if(!(Get-NetIPAddress @IPAddrParams -ErrorAction:Ignore)) {
    LogMsg "IP address not found. $($IPAddrParams | Out-String)"
    # This command creates a virtual ip for the APIPA interface
    LogMsg "Creating new virtual network adapter ip..."
    $newIpOutput = New-NetIPAddress @IPAddrParams
    LogMsg "Virtual network adapter ip created: $($newIpOutput)"
    LogMsg "Waiting for it to become available on the device..."

    [int]$delay = 1
    [int]$maxTries = 10
    [bool]$available = $false
    While (-not $available -and $maxTries -ge 0) {
        try {
            $IPAddr = Get-NetIPAddress @IPAddrParams
            if ($IPAddr) {
                $available = $true
                Break;
            }
        } catch {
            # Prevent race condition where the adapter has multiple addresses before our new ip is assigned
            # Note: Allowing this race condition can cause the netsh interface portproxy setup to  result
            #       in the network adapter in an unrecoverable bad state where the proxy is unable to
            #       listen on port 80 because the port is in use by a non-functional proxy rule.
        } finally {
            LogMsg "Waiting for ip $($CREDENTIAL_ADDRESS) to become available on interface index $($InterfaceIndex)... (sleeping $($delay))"
            sleep $delay
            $maxTries--
        }
    }
    
}
if ($available -eq $false) {
    [string]$msg = "There was a problem getting the net ip address for the virtual adapter"
    LogMsg "ERROR: $msg"
    throw $msg
    return
} else {
    LogMsg "IP address available. $($IPAddrParams | Out-String)"
}

$NetRouteParams = @{
    DestinationPrefix = $dockerSubnet;
    InterfaceIndex = $InterfaceIndex;
}
if (-not (Get-NetRoute @NetRouteParams)) {
    LogMsg "Route not found. $($NetRouteParams  | Out-String)"
    # Enable the default docker IP range to be routable by the APIPA interface.
    LogMsg "Setting up new route for $($dockerSubnet) on adapter $($InterfaceIndex)..."
    $newRouteOutput = New-NetRoute @NetRouteParams
    LogMsg "Route setup complete: $($newRouteOutput)"
}
if (-not (Get-NetRoute @NetRouteParams)) {
    [string]$msg = "Route not found. $($NetRouteParams  | Out-String)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
}

$NewRuleParam = @{
    Action = 'Allow';
    DisplayName = "Allow Inbound Port $CREDENTIAL_CONNECT_PORT";
    Direction = 'Inbound';
    LocalPort = $CREDENTIAL_CONNECT_PORT;
    Protocol = 'TCP';
}
if (-not (Get-NetFirewallRule | ?{   $_.Action -eq $NewRuleParam.Action `
                                -and $_.Direction -eq $NewRuleParam.Direction `
                                -and $_.DisplayName -eq $NewRuleParam.DisplayName})) {
    LogMsg "Route not found.  $($NewRuleParam  | Out-String)"
    # Exposes credential port for local windows firewall
    LogMsg "Setting up new inbound firewall rule to allow $($NewRuleParam.Protocol) $($NewRuleParam.LocalPort)..."
    $newFWOutput = New-NetFirewallRule @NewRuleParam
    LogMsg "Firewall rule setup complete: $($newFWOutput)"
}
if (-not (Get-NetFirewallRule | ?{   $_.Action -eq $NewRuleParam.Action `
                                -and $_.Direction -eq $NewRuleParam.Direction `
                                -and $_.DisplayName -eq $NewRuleParam.DisplayName})) {
    [string]$msg = "Firewall Rule not found. $($NewRuleParam  | Out-String)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
}

# This forwards traffic from port 80 and listens on the IAM role IP address.
# TODO: Convert this to powershell shouyld 'netsh interface portproxy' have a powershell cmdlet/module equivalent.
$netshShowAllCmd = "netsh interface portproxy show all"
$netshReturn = Invoke-Expression -Command $netshShowAllCmd
LogMsg $netshShowAllCmd
Foreach ($line in $netshReturn) {
    LogMsg $line
}

LogMsg "Setting up new ipv4 interface proxy to forward traffic"
LogMsg "  from $($CREDENTIAL_ADDRESS):$($CREDENTIAL_LISTEN_PORT)"
LogMsg "  to $($CREDENTIAL_ADDRESS):$($CREDENTIAL_CONNECT_PORT)"
try {
    [string]$netshPortProxyCmd = "netsh interface portproxy add v4tov4 listenaddress=$CREDENTIAL_ADDRESS " + `
                                                                      "listenport=$CREDENTIAL_LISTEN_PORT " + `
                                                                      "connectaddress=$CREDENTIAL_ADDRESS " + `
                                                                      "connectport=$CREDENTIAL_CONNECT_PORT"
    $netshReturn = Invoke-Expression -Command $netshPortProxyCmd
} catch {
    [string]$msg = "There was a problem setting up the traffic forwarding. Exception: $($_.Exception.Message)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
}
Foreach ($line in $netshReturn) {
    LogMsg $line
}

LogMsg "Checking port forwarding..."
try {
    $result = Test-NetConnection -ComputerName $CREDENTIAL_ADDRESS -Port $CREDENTIAL_LISTEN_PORT
} catch {
    [string]$msg = "There was a problem validating TPC port forwarding on $($CREDENTIAL_ADDRESS):$($CREDENTIAL_LISTEN_PORT). Exception: $($_.Exception.Message)"    
    LogMsg "ERROR: $msg"
    throw $msg
} finally {
    $netshReturn = Invoke-Expression -Command $netshShowAllCmd
    LogMsg $netshShowAllCmd
    Foreach ($line in $netshReturn) {
        LogMsg $line
    }
}
if (-Not $result -or $result.TcpTestSucceeded -ne "True") {
    [string]$msg = "There was a problem validating TPC port forwarding on $($CREDENTIAL_ADDRESS):$($CREDENTIAL_LISTEN_PORT)"
    LogMsg "ERROR: $msg"
    throw $msg
    return
} else {
    LogMsg "TcpTestSucceeded: $($result.TcpTestSucceeded)"
}
LogMsg "Port forwarding setup complete."

$ErrorActionPreference=$oldActionPref
LogMsg "Host setup complete."
