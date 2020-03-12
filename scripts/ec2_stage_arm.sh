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

AWS_PROFILE=""
AWS_REGION="us-east-1"
AMI_ID="ami-053b2a8c2f3e87928" #amzn2-ami-hvm-2.0.20181020.0-aarch64-gp2
ARTIFACT_BUCKET=""
SOURCE_BUCKET=""
KEY_NAME=""
SECURITY_GROUP=""
INSTANCE_PROFILE=""

source $(dirname "${0}")/publishing-common.sh

usage() {
	echo "Usage: ${0} -a ARTIFACT_BUCKET -t SOURCE_BUCKET -k SSH_KEY -s SECURITY_GROUP -i IAM_ROLE [OPTIONS]"
	echo
	echo "This script stages a new version of the Amazon EC2 Container Agent"
	echo "built on a freshly-launched EC2 instance."
	echo "To run this script, you must have the following resources:"
	echo "* A local AWS profile with permissions for \"ec2:run-instances\","
	echo "  \"ec2:describe-instances\", \"s3:PutObject\", and \"s3:ListObjects\""
	echo "* An S3 bucket for the source"
	echo "* An S3 bucket for the artifact destination"
	echo "* An IAM profile with permissions to read from and write to the"
	echo "  source bucket (\"s3:GetObject\", \"s3:PutObject\", and \"s3:PutObjectAcl\") "
	echo "  and write to the destination S3 bucket (\"s3:PutObject\" and "
	echo "  \"s3:PutObjectAcl\")"
	echo
	echo "Options"
	echo "  -d  true|false      Dryrun (default is true)"
	echo "  -p  PROFILE         AWS CLI Profile (default is none)"
	echo "  -a  BUCKET          Artifact (Destination) AWS S3 Bucket"
	echo "  -t  BUCKET          Source AWS S3 Bucket"
	echo "  -k  SSH_KEY         SSH key name"
	echo "  -s  SECURITY_GROUP  Security group name"
	echo "  -i  IAM_ROLE        IAM Role name"
	echo "  -r  Region          Region for EC2 instance launch"
	echo "  -h                  Display this help message"
}

while getopts ":d:p:a:t:k:s:i:r:h" opt; do
	case ${opt} in
		d)
			if [[ "${OPTARG}" = "false" ]]; then
				DRYRUN=false
			else
				DRYRUN=true
			fi
			;;
		p)
			AWS_PROFILE="${OPTARG}"
			;;
		a)
			ARTIFACT_BUCKET="${OPTARG}"
			;;
		t)
			SOURCE_BUCKET="${OPTARG}"
			;;
		k)
			KEY_NAME="${OPTARG}"
			;;
		s)
			SECURITY_GROUP="${OPTARG}"
			;;
		i)
			INSTANCE_PROFILE="${OPTARG}"
			;;
		r)
		    AWS_REGION="${OPTARG}"
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

if [[ -z "${ARTIFACT_BUCKET}" ]] \
	|| [[ -z "${KEY_NAME}" ]] \
	|| [[ -z "${INSTANCE_PROFILE}" ]] \
	|| [[ -z "${SOURCE_BUCKET}" ]]; then
	usage
	exit 1
fi


commit=$(git rev-parse HEAD)
clone_url=$(git config --get remote.origin.url)
echo "Packaging ${commit} for staging..."
echo "Clone url ${clone_url}"
agent_dir=$(pwd)
s3_source_prefix="s3://${SOURCE_BUCKET}/$(hostname)-${RANDOM}"

ec2_work_root="/var/lib/ec2-stage"
ec2_gopath="${ec2_work_root}/go"
ec2_build_root="${ec2_gopath}/src/github.com/aws"

userdata=$(cat << EOU
#!/bin/bash

mkdir -p ${ec2_work_root}
exec > >(tee -a ${ec2_work_root}/userdata.log) 2>&1

yum -y install docker golang git zip unzip aws-cli

systemctl start docker --now

mkdir -p ${ec2_build_root}
export GOPATH=${ec2_gopath}
cd ${ec2_build_root}

echo "Cloning...."
git clone ${clone_url}
cd amazon-ecs-agent
git checkout ${commit}

make get-deps

scripts/stage_arm.sh -b ${ARTIFACT_BUCKET} -c ${commit} -d false
aws s3 cp "${ec2_work_root}/userdata.log" "${s3_source_prefix}/userdata.log"
shutdown -hP now
EOU
)

encoded_user_data=$(base64 <<< "${userdata}")

profile=""
if [[ -n "${AWS_PROFILE}" ]]; then
	profile="--profile=${AWS_PROFILE}"
fi

echo "Starting an EC2 instance with the following user-data:"
echo "======================================================"
echo "${userdata}"
echo "======================================================"
echo

ec2_instance_id=$(dryval aws ${profile} "--region=${AWS_REGION}" \
	ec2 run-instances \
	"--count=1" \
	"--image-id=${AMI_ID}" \
	"--key-name=${KEY_NAME}" \
	"--security-groups=${SECURITY_GROUP}" \
	"--instance-type=a1.xlarge" \
	"--instance-initiated-shutdown-behavior=terminate" \
	"--iam-instance-profile=Name=${INSTANCE_PROFILE}" \
	"--query=Instances[0].InstanceId" \
	"--output=text" \
	"--user-data=${userdata}" )

if ${DRYRUN} ; then
	ec2_instance_id="dryrun-ec2-instance-id"
fi

echo "Waiting for ${ec2_instance_id} to start..."
dryval aws ${profile} "--region=${AWS_REGION}" \
	"ec2" "wait" "instance-running" "--instance-ids=${ec2_instance_id}"

echo "Waiting for ${ec2_instance_id} to terminate..."
dryval aws ${profile} "--region=${AWS_REGION}" \
	"ec2" "wait" "instance-terminated" "--instance-ids=${ec2_instance_id}"
