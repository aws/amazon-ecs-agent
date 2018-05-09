#!/bin/bash
# Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	http:#aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

set -ex

region=$(curl -s 169.254.169.254/latest/meta-data/placement/availability-zone/ | grep -o ".*[0-9]")
accountID=$(curl -s 169.254.169.254/latest/dynamic/instance-identity/document | grep accountId | grep -oE "[0-9]*")
eval $(aws ecr get-login --region ${region} --no-include-email)
repository=${accountID}.dkr.ecr.${region}.amazonaws.com/executionrole:fts

# Create the repository if it does not exist
if ! aws ecr describe-repositories --region ${region}| grep -q 'repositoryName": "executionrole"' ;
then
    aws ecr create-repository --region ${region} --repository-name executionrole
fi

# Upload the image if it does not exist
if ! aws ecr list-images --repository-name executionrole --region ${region} | grep -q '"imageTag": "fts"' ;
then
    docker build -t ${repository} .
    docker push ${repository}
    docker rmi ${repository}
fi
