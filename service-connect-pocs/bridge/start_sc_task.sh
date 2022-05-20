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

scConfig=$(cat bridge_default_sc_conf.json)

scConfig=$(echo $scConfig | jq "@json")
overridesJson=$(jq ".containerOverrides[0].environment[0].value=${scConfig}  \
  | .containerOverrides[0].environment[1].value=\"${serverPort}\" \
  | .containerOverrides[1].environment[0].value=\"${serviceName}\" \
  | .containerOverrides[1].environment[1].value=\"${serverPort}\" \
  | .containerOverrides[1].environment[2].value=\"${otherServerPort}\"" task_overrides.json)

aws ecs run-task \
--task-definition sc-generic-server-bridge \
--cluster  arn:aws:ecs:us-west-2:624830224597:cluster/ServiceConnect \
--overrides "${overridesJson}"