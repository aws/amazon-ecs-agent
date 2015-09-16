#!/bin/bash
# Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
export GOPATH="${TOPWD}/ecs-init/Godeps/_workspace:${BUILDDIR}"
export SRCPATH="${BUILDDIR}/src/github.com/aws/amazon-ecs-init"
mkdir -p "${SRCPATH}"
ln -s "${TOPWD}/ecs-init" "${SRCPATH}"
cd "${SRCPATH}/ecs-init"
if [[ "$1" == "dev" ]]; then
	go build -tags 'development' -o "${TOPWD}/amazon-ecs-init"
else
	tags=""
	if [[ "$1" != "" ]]; then
		tags="-tags '$1'"
	fi
	CGO_ENABLED=0 go build -a ${tags} -x -ldflags '-s' -o "${TOPWD}/amazon-ecs-init"
fi
rm -r "${BUILDDIR}"
