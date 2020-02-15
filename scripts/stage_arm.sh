#!/bin/bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

set -e

DRYRUN=true
SKIP_REBUILD=false

S3_BUCKET=""
S3_ACL_OVERRIDE=""
COMMIT_ID=""

source $(dirname "${0}")/publishing-common.sh
TARGET_OS=linux

usage() {
	echo "Usage: ${0} -b BUCKET [OPTIONS]"
	echo
	echo "This script is responsible for staging new versions of the Amazon ECS Container Agent."
	echo "1. (Optionally) Push the image to a registry, tagged with :latest, :VERSION, and :SHA"
	echo "2. Push the image (and its md5sum) to S3 with -latest, -VERSION, and -SHA"
	echo
	echo "Options"
	echo "  -d  true|false  Dryrun (default is true)"
	echo "  -b  BUCKET      AWS S3 Bucket"
	echo "  -a  ACL         AWS S3 Object Canned ACL (default is public-read)"
	echo "  -i  IMAGE       Docker image name"
	echo "  -s              Skip re-build"
	echo "  -h              Display this help message"
}

stage_s3() {
	tarball="$(mktemp)"
	tarball_md5="$(mktemp)"
	tarball_manifest="$(mktemp)"

	trap "rm -r \"${tarball}\" \"${tarball_md5}\" \"${tarball_manifest}\"" RETURN EXIT

	generate_manifest ${IMAGE_TAG_VERSION} > "${tarball_manifest}"

	docker save "amazon/amazon-ecs-agent:latest" > "${tarball}"
	md5sum "${tarball}" | sed 's/ .*//' > "${tarball_md5}"
	echo "Saved with md5sum $(cat ${tarball_md5})"

    mkdir -p out
	for tag in ${IMAGE_TAG_VERSION} ${IMAGE_TAG_SHA} ${IMAGE_TAG_LATEST}; do
		echo "Publishing as ecs-agent-arm64-${tag}"
		cp "${tarball}" "out/ecs-agent-arm64-${tag}.tar"
		cp "${tarball_md5}" "out/ecs-agent-arm64-${tag}.tar.md5"
		cp "${tarball_manifest}" "out/ecs-agent-arm64-${tag}.tar.json"

	done

    zip -r artifacts_arm.zip out
    dryval aws s3 cp artifacts_arm.zip "s3://${S3_BUCKET}/${COMMIT_ID}/artifacts_arm.zip"
}


generate_manifest() {
	local version=$1

	echo "{\"agentVersion\":\"${version}\"}"
}

while getopts "c:d:b:a:sh" opt; do
	case ${opt} in
	    c)
	    	COMMIT_ID="${OPTARG}"
			;;
		d)
			if [[ "${OPTARG}" = "false" ]]; then
				DRYRUN=false
			else
				DRYRUN=true
			fi
			;;
		b)
			S3_BUCKET="${OPTARG}"
			;;
		a)
			S3_ACL_OVERRIDE="${OPTARG}"
			;;
		s)
			SKIP_REBUILD=true
			;;
		\?)
			echo "Invalid option -${OPTARG}" >&2
			usage
			exit 1
			;;
		:)
			echo "Option -${OPTARG} requires an argument." >&2
			usage
			exit 1
			;;
		h)
			usage
			exit 0
			;;
	esac
done

if [[ -z "${S3_BUCKET}" ]]; then
	usage
	exit 1
fi

if ! $(${SKIP_REBUILD}); then
    TARGET_OS="${TARGET_OS}" make release
fi

stage_s3
