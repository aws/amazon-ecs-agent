# Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

# A Dockerfile to run all the the Amazon Elastic Container Service's Container
# Agent's tests.
#
# Because the Agent's tests include starting docker containers, it is necessary
# to have both go and docker available in the testing environment.
# It's easier to get go, so start with docker-in-docker and add go on top
FROM golang:1.9
MAINTAINER Amazon Web Services, Inc.

RUN mkdir -p /go/src/github.com/aws/
WORKDIR /go/src/github.com/aws/amazon-ecs-agent

ENTRYPOINT /go/src/github.com/aws/amazon-ecs-agent/scripts/test
