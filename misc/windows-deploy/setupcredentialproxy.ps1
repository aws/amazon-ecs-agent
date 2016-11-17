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

try {
    docker run --name ecs-cred-proxy -d -p 80:51679 amazon/amazon-ecs-credential-proxy | out-null
    $ip = Get-NetIPAddress `
      | Where-Object -FilterScript { `
        ($_.InterfaceAlias  -NotMatch "vEthernet") `
        -and ($_.InterfaceAlias  -NotMatch "Loopback") `
        -and ($_.AddressFamily  -Match "IPv4") `
        -and ($_.IPAddress  -NotMatch "169.254.170.2") `
    } `
    | Sort-Object -Property "InterfaceIndex"
  $ipaddr = $ip[0].IPaddress
  netsh advfirewall firewall set rule name=all protocol=tcp localport=51679 new remoteip=$ipaddr
} catch {
    exit 2
}
