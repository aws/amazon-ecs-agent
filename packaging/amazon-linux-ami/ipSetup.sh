#!/bin/bash

set -ex


sysctl -w net.ipv4.conf.all.route_localnet=1
sysctl -e -w net.ipv6.conf.docker0.accept_ra=0
iptables -t nat -A PREROUTING -p tcp -d 169.254.170.2 --dport 80 -j DNAT --to-destination 127.0.0.1:51679

if [ -n "${ECS_SKIP_LOCALHOST_TRAFFIC_FILTER}" ]
then 
    if [ "${ECS_SKIP_LOCALHOST_TRAFFIC_FILTER}" != "true" ]
    then
        iptables -t filter -I INPUT --dst 127.0.0.0/8 ! --src 127.0.0.0/8 -m conntrack ! --ctstate RELATED,ESTABLISHED,DNAT -j DROP
    fi
else
    iptables -t filter -I INPUT --dst 127.0.0.0/8 ! --src 127.0.0.0/8 -m conntrack ! --ctstate RELATED,ESTABLISHED,DNAT -j DROP
fi

if [ -n "${ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS}" ]
then
    if [ "${ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS}" != "true" ] 
    then
        iptables -t filter -I INPUT -p tcp -i eth0 --dport 51678 -j DROP
    fi
else
    iptables -t filter -I INPUT -p tcp -i eth0 --dport 51678 -j DROP
fi

iptables -t nat -A OUTPUT -p tcp -d 169.254.170.2 --dport 80 -j REDIRECT --to-ports 51679