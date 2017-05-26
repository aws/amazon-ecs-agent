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

if [ -d "${TOPWD}/.git" ]; then
    version=$(cat "${TOPWD}/ecs-init/VERSION")
    git_hash=$(git rev-parse --short HEAD)
    git_dirty=false

    if [[ "$(git status --porcelain)" != "" ]]; then
	git_dirty=true
    fi

    VERSION_FLAG="-ldflags -X github.com/aws/amazon-ecs-init/ecs-init/version.Version=${version}"
    GIT_HASH_FLAG="-ldflags -X github.com/aws/amazon-ecs-init/ecs-init/version.GitShortHash=${git_hash}"
    GIT_DIRTY_FLAG="-ldflags -X github.com/aws/amazon-ecs-init/ecs-init/version.GitDirty=${git_dirty}"
fi

mkdir -p "${SRCPATH}"
ln -s "${TOPWD}/ecs-init" "${SRCPATH}"
cd "${SRCPATH}/ecs-init"
if [[ "$1" == "dev" ]]; then
	go build -tags 'development' "${VERSION_FLAG} ${GIT_HASH_FLAG} ${GIT_DIRTY_FLAG}" \
	   -o "${TOPWD}/amazon-ecs-init"
else
	tags=""
	if [[ "$1" != "" ]]; then
		tags="-tags '$1'"
	fi
	CGO_ENABLED=0 go build -a ${tags} -x -ldflags '-s' \
		   ${VERSION_FLAG} ${GIT_HASH_FLAG} ${GIT_DIRTY_FLAG} \
		   -o "${TOPWD}/amazon-ecs-init"
fi
rm -r "${BUILDDIR}"
