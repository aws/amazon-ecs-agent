#!/bin/bash

set -ex

dependency=${5:-"dependency"}

usage()
{
    echo "usage: start_sc_task.sh --network awsvpc|bridge --sc-mode default|nondefault --service service_name --port 8080 --other-port 9090 --dependencies auth,db"
}

if [ -z $1 ]
then
  usage
  exit 1
fi

while [ "$1" != "" ]; do
    case $1 in
        -n | --network )       shift
                                networkMode=$1
                                ;;
        -s | --service )        shift
                                serviceName=$1
                                ;;
        -p | --port )           shift
                                serverPort=$1
                                ;;
        -op | --other-port )    shift
                                otherServerPort=$1
                                ;;
        -d | --dependencies )   shift
                                dependencies=$1
                                ;;
        -i | --sc-ingress )     shift
                                scIngress=$1
                                ;;
        * )                     usage
                                exit 1
    esac
    shift
done

if [ "${scIngress}" == "0" ] # Default SC experience
then
  scConfig=$(jq ".ingressConfig[0] += {\"interceptPort\":${serverPort}}" awsvpc_default_sc_conf.json)
else
  scConfig=$(jq ".dnsConfig[0].hostName=\"${dependencies}.my.corp\" | .ingressConfig[0] += {\"listenerPort\":${scIngress}}" awsvpc_default_sc_conf.json)
fi

if [ "${dependencies}" != "none" ]
then
  IFS=',' read -r -a deps <<< "${dependencies}"
  for i in "${!deps[@]}"; do
    IFS=':' read -r -a depElems <<< "${deps[$i]}"
    scConfig=$(echo $scConfig | jq ".dnsConfig[${i}].hostName=\"${depElems[0]}\"")
  done

  scConfig=$(echo $scConfig | jq "@json")
  overridesJson=$(jq ".containerOverrides[0].environment[0].value=${scConfig}  | .containerOverrides[0].environment[1].value=\"${serverPort}\" | .containerOverrides[1].environment[0].value=\"${serviceName}\" | .containerOverrides[1].environment[1].value=\"${serverPort}\" | .containerOverrides[1].environment[2].value=\"${otherServerPort}\" | .containerOverrides[1].environment[3].value=\"${dependencies}\"" task_overrides.json)

  curEnvIdx=2
  for i in "${!deps[@]}"; do
    IFS=':' read -r -a depElems <<< "${deps[$i]}"
    echo $i
    echo $curEnvIdx
    overridesJson=$(echo "${overridesJson}" | jq ".containerOverrides[0].environment[$((curEnvIdx+i))].value=\"${depElems[1]}\"")
    overridesJson=$(echo "${overridesJson}" | jq ".containerOverrides[0].environment[$((curEnvIdx+i+1))].value=\"${depElems[2]}\"")
    curEnvIdx=$((curEnvIdx+1))
  done
else
  scConfig=$(echo $scConfig | jq "@json")
  overridesJson=$(jq ".containerOverrides[0].environment[0].value=${scConfig}  | .containerOverrides[0].environment[1].value=\"${serverPort}\" | .containerOverrides[1].environment[0].value=\"${serviceName}\" | .containerOverrides[1].environment[1].value=\"${serverPort}\" | .containerOverrides[1].environment[2].value=\"${otherServerPort}\"" task_overrides.json)
fi


aws ecs run-task \
--task-definition sc-generic-server-awsvpc \
--cluster 3B6B8EC9-4640-41E3-8761-023F76B07364 \
--network-configuration "awsvpcConfiguration={subnets=[subnet-0aa34b0e7e4881fee],securityGroups=[sg-0604036f1bbb336ec]}" \
--overrides "${overridesJson}"