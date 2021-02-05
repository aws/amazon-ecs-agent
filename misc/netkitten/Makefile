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
.PHONY: all clean image

GO_VERSION=$(shell cat ../../GO_VERSION)

all: image

image:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t "amazon/amazon-ecs-netkitten:make" -f ./Dockerfile.linux .

clean:
	-docker rmi -f "amazon/amazon-ecs-netkitten:make"
