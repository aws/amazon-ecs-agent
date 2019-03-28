# Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
.PHONY: all clean elastic-inference-validator image

all: appmesh-plugin-validator image

appmesh-plugin-validator: appmesh-plugin-validator.go
	@./build-in-docker

image: appmesh-plugin-validator
	docker build -t amazon/amazon-appmesh-plugin-validator:make .

clean:
	rm -f appmesh-plugin-validator
	-docker rmi -f "amazon/amazon-appmesh-plugin-validator:make"