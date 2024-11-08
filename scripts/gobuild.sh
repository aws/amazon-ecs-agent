#!/bin/bash
# Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the
# "License"). You may not use this file except in compliance
#  with the License. A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.
set -x
set -e
export TOPWD="$(pwd)"
export BUILDDIR="$(mktemp -d)"
export SRCPATH="${BUILDDIR}/src/github.com/aws/amazon-ecs-agent"
export GOPATH="${TOPWD}:${BUILDDIR}"
export GO111MODULE="auto"

# we need to make sure we've got the correct golang version installed
# if it's already installed the script will set env vars and exit
source ./scripts/install-golang.sh

# regenerate version files for accurate version info
./scripts/version-gen.go

mkdir -p "${SRCPATH}"
ln -s "${TOPWD}/ecs-init" "${SRCPATH}"
cd "${SRCPATH}/ecs-init"
if [[ "$1" == "dev" ]]; then
	CGO_ENABLED=1 CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' go build \
        -o "${TOPWD}/amazon-ecs-init"
else
	tags=""
	if [[ "$1" != "" ]]; then
		tags="-tags '$1'"
	fi
	CGO_ENABLED=1 CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' go build -a ${tags} -x \
        -o "${TOPWD}/amazon-ecs-init"
fi
CGO_ENABLED=0 go build -x -o "${TOPWD}/amazon-ecs-volume-plugin" "./volumes/amazon-ecs-volume-plugin"
rm -r "${BUILDDIR}"
