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
set -ex
ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )
cd "${ROOT}"

cd ../..

export TOPWD="$(pwd)"

if [ -d "${TOPWD}/.git" ]; then
    version=$(cat "${TOPWD}/VERSION")
    git_hash=$(git rev-parse --short=8 HEAD)
    git_dirty=false

    if [[ "$(git status --porcelain)" != "" ]]; then
	git_dirty=true
    fi

    VERSION_FLAG="-X github.com/aws/amazon-ecs-agent/agent/version.Version=${version}"
    GIT_HASH_FLAG="-X github.com/aws/amazon-ecs-agent/agent/version.GitShortHash=${git_hash}"
    GIT_DIRTY_FLAG="-X github.com/aws/amazon-ecs-agent/agent/version.GitDirty=${git_dirty}"
fi


CGO_ENABLED=0 go build -ldflags "-s ${VERSION_FLAG} ${GIT_HASH_FLAG} ${GIT_DIRTY_FLAG}" \
	-o "${ROOT}/amazon-ecs-volume-plugin" "${TOPWD}/agent/volumes/amazon-ecs-volume-plugin"
