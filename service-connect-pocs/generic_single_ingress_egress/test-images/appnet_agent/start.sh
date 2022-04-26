#!/bin/bash

set -ex

ingress_port=$(echo ${SC_CONFIG} | jq -c '.ingressConfig | .[] | select (.listenerName == "ingress_listener") | .listenerPort ')

if [[ "${ingress_port}" == "null" || "${ingress_port}" == "0" ]]
then
  ingress_port=$(echo ${listener_port_mapping} | jq -c '.ingress_listener')
fi


egress_port=$(echo ${listener_port_mapping} | jq -c '.egress_listener')

intercept_port=$(echo ${SC_CONFIG} | jq -c '.ingressConfig | .[] | select (.listenerName == "ingress_listener") | .interceptPort ')
app_port=$intercept_port
if [[ "$app_port" == "null" || "$app_port" == null ]]
then
  app_port=${SC_APP_PORT}
fi


vipCidr=$(echo ${SC_CONFIG} | jq -r '.egressConfig.vip.ipv4')

_SC_INGRESS_PORT_=${ingress_port:-15000}
_SC_EGRESS_PORT_=${egress_port:-30000}
_SC_APP_PORT_=${app_port:-8080}
_SC_REMOTE_IP_1_=${SC_REMOTE_IP_1:-"127.0.0.1"}
_SC_REMOTE_PORT_1_=${SC_REMOTE_PORT_1:-11111}
_SC_REMOTE_IP_2_=${SC_REMOTE_IP_2:-"127.0.0.1"}
_SC_REMOTE_PORT_2_=${SC_REMOTE_PORT_2:-22222}

sed -i "s/SC_INGRESS_PORT/${_SC_INGRESS_PORT_}/g" "/etc/envoy/lds_config.yaml"
sed -i "s/SC_EGRESS_PORT/${_SC_EGRESS_PORT_}/g" "/etc/envoy/lds_config.yaml"
sed -i "s/SC_APP_PORT/${_SC_APP_PORT_}/g" "/etc/envoy/local_eds.yaml"
sed -i "s/127.0.0.1/${_SC_REMOTE_IP_1_}/g" "/etc/envoy/remote_eds_1.yaml"
sed -i "s/11111/${_SC_REMOTE_PORT_1_}/g" "/etc/envoy/remote_eds_1.yaml"
sed -i "s/11111/${_SC_REMOTE_PORT_1_}/g" "/etc/envoy/lds_config.yaml"
sed -i "s/127.0.0.1/${_SC_REMOTE_IP_2_}/g" "/etc/envoy/remote_eds_2.yaml"
sed -i "s/22222/${_SC_REMOTE_PORT_2_}/g" "/etc/envoy/remote_eds_2.yaml"
sed -i "s/22222/${_SC_REMOTE_PORT_2_}/g" "/etc/envoy/lds_config.yaml"

# Setup egress rules awsvpc mode
iptables -t nat -A OUTPUT -p tcp \
  -d ${vipCidr} -j REDIRECT --to-port ${_SC_EGRESS_PORT_}

# setup ingress rules
if [ "$intercept_port" != "null" ]
then
  iptables -t nat -A PREROUTING -p tcp \
    -m multiport -m addrtype ! --src-type LOCAL \
    --dports ${intercept_port} -j REDIRECT \
    --to-port ${_SC_INGRESS_PORT_} # to SC ingress port
fi

/usr/bin/envoy -c /etc/envoy/envoy.yaml -l debug