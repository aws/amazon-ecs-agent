# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

ARG GO_VERSION
FROM public.ecr.aws/docker/library/golang:${GO_VERSION}

ENV XDG_CACHE_HOME /tmp

RUN go env -w GOPROXY=direct

RUN apt-get update && \
    apt-get install -y --no-install-recommends openjdk-17-jdk

RUN mkdir -p /go/src/github.com/aws/

RUN git clone https://github.com/aws/aws-sdk-go-v2.git /go/src/github.com/aws/aws-sdk-go-v2

WORKDIR /go/src/github.com/aws/aws-sdk-go-v2

# Remove test codegen directories to speed up the SDK codegen process.
RUN rm -r codegen/protocol-test-codegen codegen/smithy-aws-go-codegen/src/test

RUN make smithy-go-publish-local

CMD ["make", "smithy-generate", "update-requires", "update-module-metadata", "min-go-version-service_.", "tidy-modules-service_."]
