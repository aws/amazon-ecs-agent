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
.PHONY: all image mockgen clean

# 'go' may not be on the $PATH for sudo tests
GO_EXECUTABLE=$(shell command -v go 2> /dev/null)
CSI_DRIVER_VERSION="v$(shell cat CSI_DRIVER_VERSION)"
BUILD_DATE=$(shell date -u -Iseconds)
GO_VERSION=$(shell cat ../../../GO_VERSION)
IMAGE_REF?="ebs-csi-driver:latest"
# -count=1 runs the test without using the build cache.  The build cache can
# provide false positives when running integ tests, so we err on the side of
# caution. See `go help test`
# unit tests include the coverage profile
GOTEST=${GO_EXECUTABLE} test -count=1

all: tarfiles/ebs-csi-driver.tar

bin/ebs-csi-driver:
	CGO_ENABLED=0 go build -ldflags "\
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/version.version=$(CSI_DRIVER_VERSION)\" \
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/version.buildDate=$(BUILD_DATE)\"" \
		-o bin/ebs-csi-driver .

image:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t $(IMAGE_REF) .

test:
	GO111MODULE=on ${GOTEST} -v -race ./driver/... -tags unit -mod vendor -timeout=120s

tarfiles/ebs-csi-driver.tar: image
	mkdir -p tarfiles
	docker save $(IMAGE_REF) > tarfiles/ebs-csi-driver.tar

mockgen:
	./update-gomock

clean:
	-docker rmi -f $(IMAGE_REF)
	-rm -rf tarfiles/
	-rm -rf bin/
