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
.PHONY: all clean container-metadata-file-validator image

all: container-metadata-file-validator image

container-metadata-file-validator: container-metadata-file-validator.go
	@./build-in-docker

image: container-metadata-file-validator
	docker build -t amazon/amazon-ecs-container-metadata-file-validator:make .

clean:
	rm -f taskmetadata-validator
	-docker rmi -f "amazon/amazon-ecs-container-metadata-file-validator:make"
