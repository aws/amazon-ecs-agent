#!/usr/bin/env bash
# Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#   http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.
#
# The script is used to spin up an EC2 windows instance and run SSM command
# on it. The command runs the generate_images.ps1 to generate windows docker
# images used by agent integration/functional tests and upload them to ECR.

set -euo pipefail

DRYRUN=true

AWS_PATH="/apollo/env/AmazonAwsCli/bin/aws"
AWS_REGION="us-west-2"
JQ_PATH="jq"
GENERATE_IMAGE_FILE_NAME="generate_image.ps1"
GIT_REPO="https://github.com/aws/amazon-ecs-agent.git"
TIMEOUT="7200"
WIN_AMI_ID=""


SECURITY_GROUP="default"
SHA=""
KEY_PAIR_NAME=""
INSTANCE_PROFILE=""
CODE_BUCKET=""
OUTPUT_BUCKET=""
S_REPOSITORY=""
R_REPOSITORY=""
S_REGION=""
R_REGION=""

dryval() {
    if "${DRYRUN}"; then
        echo "DRYRUN: ${@}" 1>&2
    else
        echo "RUNNING: ${@}" 1>&2
        "${@}"
    fi
}

error_exit() {
    local error_msg="$1"
    echo "${error_msg}" 1>&2
    exit 1
}

usage() {
    cat <<-EOU
Usage: ${0} --key-pair KEY_PAIR_NAME --sha SHA --instance-profile INSTANCE_PROFILE --code-bucket CODE_BUCKET \
--output-bucket OUTPUT_BUCKET --standard-repo S_REPOSITORY --replication-repo R_REPOSITORY --standard-region \
S_REGION --replication-region R_REGION --security-group=SECURITY_GROUP --dryrun=DRYRUN[OPTIONS]

This script is used for generating windows images used by agent functional tests and replicating images to BJS region.

Options
  --key-pair  KEY_PAIR_NAME                        Key pair name for the EC2 instance
  --sha  SHA                                       SHA of the commit of agent code you want to checkout
  --instance-profile  INSTANCE_PROFILE             Instance profile for EC2 instance
  --security-group  SECURITY_GROUP                 Security group for EC2 instance(optional)
  --code-bucket  CODE_BUCKET                       S3 bucket storing generate_image.ps1 code file
  --output-bucket  OUTPUT_BUCKET                   S3 bucket storing ssm run commands output
  --standard-repo  S_REPOSITORY                    ECR repository URI in standard regions
  --replication-repo  R_REPOSITORY                 ECR repository URI in regions that replicating images to
  --standard-region  S_REGION                      The standard region that ECR repository in
  --replication-region  R_REGION                   The replication region that ECR repository in
  --dryrun  DRYRUN                                 Dryrun: true or false (optional)
  --help                                           Display this help message
EOU
}

generate_windows_images() {
    echo
    echo "======================================"
    echo "Generating windows images..."
    echo "======================================"
    echo
    local tmp_file="$(mktemp)"

    trap "rm -f ${tmp_file}" RETURN EXIT

    local ec2_instance_id=$(dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 run-instances \
        "--count=1" \
        "--image-id=${WIN_AMI_ID}" \
        "--key-name=${KEY_PAIR_NAME}" \
        "--security-groups=${SECURITY_GROUP}" \
        "--instance-type=m4.2xlarge" \
        "--instance-initiated-shutdown-behavior=terminate" \
        "--iam-instance-profile=Name=${INSTANCE_PROFILE}" \
        "--query=Instances[0].InstanceId" \
        "--output=text")

    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 create-tags "--resources=${ec2_instance_id}" \
        "--tags=Key=GenerateWindowsImages,Value=${SHA}"

    echo "Waiting for ${ec2_instance_id} to start..."
    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 wait instance-running "--instance-ids=${ec2_instance_id}"

    echo "Waiting for ${ec2_instance_id} to be status ok..."
    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 wait instance-status-ok "--instance-ids=${ec2_instance_id}"

    while ! "${DRYRUN}"; do
        local status=$(dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
            ssm describe-instance-information --instance-information-filter-list="key=InstanceIds,valueSet=${ec2_instance_id}" --query 'InstanceInformationList[0]')

        if [[ "${status}" == "null" ]]; then
            echo "Instance has not been registered in SSM"
            sleep 30
        else
            echo "Instance has registered in SSM"
            break
        fi
    done

    echo "Calling SSM to run ${GENERATE_IMAGE_FILE_NAME} on ${ec2_instance_id}..."

    local ssm_commands="[Start-BitsTransfer -Source https://${CODE_BUCKET}.s3.amazonaws.com/${GENERATE_IMAGE_FILE_NAME} \
        -Destination ./${GENERATE_IMAGE_FILE_NAME}, \
        ./${GENERATE_IMAGE_FILE_NAME} -GIT_REPO ${GIT_REPO} -COMMIT_SHA ${SHA} \
        -S_REGION ${S_REGION} -R_REGION ${R_REGION} \
        -S_REPOSITORY ${S_REPOSITORY} -R_REPOSITORY ${R_REPOSITORY}]"

    local ssm_commands_res=$(dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ssm send-command \
        "--document-name=AWS-RunPowerShellScript" \
        "--comment=Generate Windows Images" \
        "--timeout-seconds=${TIMEOUT}" \
        "--instance-ids=${ec2_instance_id}" \
        --parameters "commands=${ssm_commands},executionTimeout=[${TIMEOUT}]" \
        "--output-s3-bucket-name=${OUTPUT_BUCKET}")

    local ssm_command_id=$(echo "${ssm_commands_res}" | "${JQ_PATH}" -r '.Command.CommandId')

    echo "waiting for SSM command ${ssm_command_id} to be finished..."
    while ! "${DRYRUN}"; do
        local status=$(dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
            ssm list-commands "--command-id=${ssm_command_id}" | "${JQ_PATH}" -r '.Commands[0].Status')

        if [[ "${status}" == "Success" ]]; then
            echo "SSM command is executed successfully"
            break
        elif [[ "${status}" == "Pending" || "${status}" == "InProgress" || "${status}" == "Delayed" ]]; then
            sleep 120
            continue
        else
            error_exit "Failed to execute SSM command"
        fi
    done

    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ssm get-command-invocation \
        "--command-id=${ssm_command_id}" \
        "--instance-id=${ec2_instance_id}" > ${tmp_file}

    local ssm_standard_error_content=$(cat "${tmp_file}" | "${JQ_PATH}" -r '.StandardErrorContent')

    if ! "${DRYRUN}"; then
        if [[ "${ssm_standard_error_content}" == "" ]]; then
            echo "Windows images are generated successfully"
        else
            error_exit "Failed to generate windows images"
        fi
    fi

    # Shut down the instance
    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 terminate-instances "--instance-ids=${ec2_instance_id}"

    echo "Waiting for ${ec2_instance_id} to terminate..."
    dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
        ec2 wait instance-terminated "--instance-ids=${ec2_instance_id}"
}

OPTS=`getopt --options "" \
    --long "key-pair:,sha:,instance-profile:,security-group::,code-bucket:,output-bucket:, \
    standard-repo:,replication-repo:,standard-region:,replication-region:,dryrun::,help" -- "$@"`

if [ $? -ne 0 ]; then
    error_exit "Incorrect options provided"
fi

eval set -- "${OPTS}"

while true; do
    case "$1" in
        --key-pair)
            KEY_PAIR_NAME="$2"
            shift 2
            ;;
        --sha)
            SHA="$2"
            shift 2
            ;;
        --instance-profile)
            INSTANCE_PROFILE="$2"
            shift 2
            ;;
        --code-bucket)
            CODE_BUCKET="$2"
            shift 2
            ;;
        --output-bucket)
            OUTPUT_BUCKET="$2"
            shift 2
            ;;
        --standard-repo)
            S_REPOSITORY="$2"
            shift 2
            ;;
        --replication-repo)
            R_REPOSITORY="$2"
            shift 2
            ;;
        --standard-region)
            S_REGION="$2"
            shift 2
            ;;
        --replication-region)
            R_REGION="$2"
            shift 2
            ;;
        --security-group)
            case "$2" in
                "") ;;
                *) SECURITY_GROUP="$2" ;;
            esac
            shift 2
            ;;
        --dryrun)
            case "$2" in
                "") ;;
                *)
                    if [[ "$2" == "false" ]]; then
                        DRYRUN=false
                    else
                        DRYRUN=true
                    fi
                    ;;
            esac
            shift 2
            ;;
        --help)
            usage
            exit 0
            ;;
        --)
            shift
            break
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "${KEY_PAIR_NAME}" ] \
    || [ -z "${SHA}" ] \
    || [ -z "${INSTANCE_PROFILE}" ] \
    || [ -z "${CODE_BUCKET}" ] \
    || [ -z "${OUTPUT_BUCKET}" ] \
    || [ -z "${S_REPOSITORY}" ] \
    || [ -z "${R_REPOSITORY}" ] \
    || [ -z "${S_REGION}" ] \
    || [ -z "${R_REGION}" ] \
    || [ -z "${SECURITY_GROUP}" ]; then
    usage
    exit 1
fi

# Get the windows AMI ID
WIN_AMI_ID=$(dryval "${AWS_PATH}" "--region=${AWS_REGION}" \
    ec2 describe-images --owners=amazon \
    --filters="Name=name,Values=Windows_Server-2016-English-Full-ECS_DockerBase*" \
    --query="sort_by(Images,&CreationDate)[].ImageId" | "${JQ_PATH}" -r '.[-1]')

echo "${WIN_AMI_ID}"

if [[ ! "${DRYRUN}" && (-z "${WIN_AMI_ID}" || "${WIN_AMI_ID}" == "null") ]]; then
    error_exit "Error geting Windows AMI ID"
fi

generate_windows_images