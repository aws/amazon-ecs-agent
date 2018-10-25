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

.PHONY: all container clean

TAG = make

all: container

# Tests that use namespace sharing require different images. They are all 
# grouped as namespace-tests and will be built together
container:
	docker build -f Dockerfile.pidNamespaceTest -t "amazon/amazon-ecs-pid-namespace-test:$(TAG)" .
	docker build -f Dockerfile.ipcNamespaceTest -t "amazon/amazon-ecs-ipc-namespace-test:$(TAG)" .

clean:
	docker rmi "amazon/amazon-ecs-pid-namespace-test:$(TAG)"
	docker rmi "amazon/amazon-ecs-ipc-namespace-test:$(TAG)"