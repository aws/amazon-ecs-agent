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
.PHONY: all clean

all: .docker-stamp 

.docker-stamp:
	docker build -t amazon/nfs-tester:make .
	docker run -d \
	    --tmpfs /proc/fs/nfsd \
	    --tmpfs /var/lib/nfs/rpc_pipefs \
	    --privileged \
	    --net=none \
	    --name=nfs-test \
	    -v "$(PWD)/integ:/var/files" \
	    amazon/nfs-tester:make
	touch .docker-stamp

clean:
	-docker stop nfs-test
	-docker rm nfs-test
	-docker rmi -f "amazon/nfs-tester:make"
	-rm -f .docker-stamp
