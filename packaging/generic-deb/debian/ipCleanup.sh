#!/bin/bash

set -ex

sysctl net.ipv4.conf.default.route_localnet
sysctl -w net.ipv4.conf.all.route_localnet=0
iptables -t nat -D PREROUTING -p tcp -d 169.254.170.2 --dport 80 -j DNAT --to-destination 127.0.0.1:51679

if [ -n "${ECS_SKIP_LOCALHOST_TRAFFIC_FILTER}" ]
then 
    if [ "${ECS_SKIP_LOCALHOST_TRAFFIC_FILTER}" != "true" ]
    then
        iptables -t filter -D INPUT --dst 127.0.0.0/8 ! --src 127.0.0.0/8 -m conntrack ! --ctstate RELATED,ESTABLISHED,DNAT  -j DROP
    fi
else
    iptables -t filter -D INPUT --dst 127.0.0.0/8 ! --src 127.0.0.0/8 -m conntrack ! --ctstate RELATED,ESTABLISHED,DNAT  -j DROP
fi

iptables -t filter -D INPUT -p tcp -i eth0 --dport 51678 -j DROP
iptables -t nat -D OUTPUT -p tcp -d 169.254.170.2 --dport 80 -j REDIRECT --to-ports 51679