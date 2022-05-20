#!/bin/bash

set -ex

ingress_port=$(echo ${SC_CONFIG} | jq -c '.ingressConfig | .[0].listenerPort ')
egress_port=$(echo ${APPNET_LISTENER_PORT_MAPPING} | jq -c '.egress_listener')
app_port=${SC_APP_CONTAINER_PORT} # local cx app container - container port


vipCidr=$(echo ${SC_CONFIG} | jq -r '.egressConfig.vip.ipv4Cidr')

_SC_INGRESS_PORT_=${ingress_port:-15000}
_SC_EGRESS_PORT_=${egress_port:-30000}
_SC_APP_PORT_=${app_port:-8080}
_SC_APP_CONTAINER_IP_=$(echo ${APPNET_CONTAINER_MAPPING} | jq -c '.server') 
_SC_REMOTE_IP_1_=${SC_REMOTE_IP_1:-"127.0.0.1"}
_SC_REMOTE_PORT_1_=${SC_REMOTE_PORT_1:-11111}


sed -i "s/SC_INGRESS_PORT/${_SC_INGRESS_PORT_}/g" "/etc/envoy/lds_config.yaml"
sed -i "s/SC_EGRESS_PORT/${_SC_EGRESS_PORT_}/g" "/etc/envoy/lds_config.yaml"
sed -i "s/SC_APP_PORT/${_SC_APP_PORT_}/g" "/etc/envoy/local_eds.yaml"
sed -i "s/SC_APP_CONTAINER_IP/${_SC_APP_CONTAINER_IP_}/g" "/etc/envoy/local_eds.yaml" # bridge
sed -i "s/127.0.0.1/${_SC_REMOTE_IP_1_}/g" "/etc/envoy/remote_eds_1.yaml"
sed -i "s/11111/${_SC_REMOTE_PORT_1_}/g" "/etc/envoy/remote_eds_1.yaml"
sed -i "s/11111/${_SC_REMOTE_PORT_1_}/g" "/etc/envoy/lds_config.yaml"


# This emulates CNI plugin to configure tproxy for envoy netns
envoyPauseContainerId=$(/usr/bin/docker ps -q --filter "name=internalecspause-service-connect")
envoyIp=$(/usr/bin/docker inspect $envoyPauseContainerId | jq -r --arg NET_NAME "bridge" '.[0].NetworkSettings.Networks[$NET_NAME].IPAddress')

iptables -t mangle -N DIVERT
iptables -t mangle -A PREROUTING -p tcp -m socket -j DIVERT
iptables -t mangle -A DIVERT -j MARK --set-mark 1
iptables -t mangle -A DIVERT -j ACCEPT
ip rule add fwmark 1 lookup 100
ip route add local 0.0.0.0/0 dev lo table 100
iptables -t mangle -A PREROUTING -i eth0 -p tcp -m tcp -d ${vipCidr} -j TPROXY --tproxy-mark 0x1/0x1 --on-port ${egress_port}

iptables -t raw -A PREROUTING -p tcp -j LOG --log-prefix "SC pre pause-envoy-${ID} " --log-level 4 	# LOGGING
iptables -t raw -A OUTPUT -p tcp -j LOG --log-prefix "SC out pause-envoy-${ID} " --log-level 4 		# LOGGING

# This emulates CN plugin to configure ip route for SC outbound traffic
serverPauseContainerId=$(/usr/bin/docker ps -q  --filter "name=internalecspause-server")
serverPauseContainerPid=$(/usr/bin/docker inspect $serverPauseContainerId | jq -r .[0].State.Pid)

nsenter -t ${serverPauseContainerPid} -n ip route add ${vipCidr} via ${envoyIp} dev eth0
nsenter -t ${serverPauseContainerPid} -n iptables -t raw -A PREROUTING -p tcp -j LOG --log-prefix "SC pre ${cid} " --log-level 4 	# LOGGING
nsenter -t ${serverPauseContainerPid} -n iptables -t raw -A OUTPUT -p tcp -j LOG --log-prefix "SC out ${cid} " --log-level 4 		# LOGGING

/usr/bin/envoy -c /etc/envoy/envoy.yaml -l debug