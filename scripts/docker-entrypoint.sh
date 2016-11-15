#!/bin/sh
set -e

if [ "${1:0:1}" = '-' ]; then
    set -- /agent "$@"
fi

if [ "$1" = '/agent' ]; then
  if [ ! -S /var/run/docker.sock ]; then
    echo 'Docker socket not found, exiting.'
    exit 1
  fi

  # Guarantee that we can talk to docker or ecs-agent will start anyway
  # but won't be able to do anything.
  socat - UNIX-CONNECT:/var/run/docker.sock >/dev/null <<EOF
GET /info HTTP/1.1

EOF
  if [ $? -ne 0 ]; then
    echo 'Failed to connect to /var/run/docker.sock, exiting.'
    exit 1
  fi
fi

exec "$@"
