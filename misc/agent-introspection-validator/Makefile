# Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
.PHONY: all clean agent-introspection-validator image

all: agent-introspection-validator image

agent-introspection-validator: agent-introspection-validator.go
	@./build-in-docker

image: agent-introspection-validator
	docker build -t amazon/amazon-ecs-agent-introspection-validator:make .

clean:
	rm -f agent-introspection-validator
	-docker rmi -f "amazon/amazon-ecs-agent-introspection-validator:make"
