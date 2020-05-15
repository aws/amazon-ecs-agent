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
.PHONY: all clean v3-task-endpoint-validator image

all: v3-task-endpoint-validator image

v3-task-endpoint-validator: v3-task-endpoint-validator.go
	@./build-in-docker

image: v3-task-endpoint-validator
	docker build -t amazon/amazon-ecs-v3-task-endpoint-validator:make .

clean:
	rm -f taskmetadata-validator
	-docker rmi -f "amazon/amazon-ecs-v3-task-endpoint-validator:make"
