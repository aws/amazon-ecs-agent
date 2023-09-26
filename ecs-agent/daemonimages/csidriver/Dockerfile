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
FROM public.ecr.aws/docker/library/golang:${GO_VERSION} as build

WORKDIR /go/src/
ADD . /go/src/

RUN make bin/ebs-csi-driver

# use minimal eks base for csi driver image to add required mount utils
FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-csi-ebs:latest-al2

MAINTAINER Amazon Web Services, Inc.
COPY --from=build /go/src/bin/ebs-csi-driver /bin/ebs-csi-driver

ENTRYPOINT ["/bin/ebs-csi-driver"]
