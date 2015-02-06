# Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You
# may not use this file except in compliance with the License. A copy of
# the License is located at
#
# 	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF
# ANY KIND, either express or implied. See the License for the specific
# language governing permissions and limitations under the License.

.PHONY: all gobuild static checkdockerfile docker release certs test clean netkitten test-registry

all: release

# Dynamic go build; useful in that it does not have -a so it won't recompile
# everything every time
gobuild:
	cd agent && godep go build -o ../out/amazon-ecs-agent .

# Basic go build
static:
	cd agent && CGO_ENABLED=0 godep go build -a -x -ldflags '-s' -o ../out/amazon-ecs-agent .

# Phony target to make sure we never clobber someone's dockerfile. TODO, better
# cleaning up of the dockerfile we ln so that this doesn't trigger incorrectly
checkdockerfile:
	@if [ -e Dockerfile ]; then echo "Dockerfile exists; please remove or rename it before building"; exit 1; fi

docker: checkdockerfile
	@ln -s scripts/dockerfiles/Dockerfile.build Dockerfile
	docker build -t "amazon/amazon-ecs-agent-build:make" .
	docker run -v "$(shell pwd)/out:/out" -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-agent" "amazon/amazon-ecs-agent-build:make"
	@rm -f Dockerfile

# Release packages our agent into a "scratch" based dockerfile
release: checkdockerfile certs out/amazon-ecs-agent
	@cd scripts && ./create-amazon-ecs-scratch
	@ln -s scripts/dockerfiles/Dockerfile.release Dockerfile
	docker build -t "amazon/amazon-ecs-agent:make" .
	@echo "Built Docker image \"amazon/amazon-ecs-agent:make\""
	@rm -f Dockerfile

# There's two ways to build this; default to the docker way
out/amazon-ecs-agent: docker


# We need to bundle certificates with our scratch-based container
certs: misc/certs/ca-certificates.crt
misc/certs/ca-certificates.crt:
	docker build -t "amazon/amazon-ecs-agent-cert-source:make" misc/certs/
	docker run "amazon/amazon-ecs-agent-cert-source:make" cat /etc/ssl/certs/ca-certificates.crt > misc/certs/ca-certificates.crt

short-test:
	cd agent && godep go test -short -timeout=25s -v -cover ./...

# Run our 'test' registry needed for integ tests
test-registry: netkitten
	@./scripts/setup-test-registry

test: test-registry
	cd agent && godep go test -timeout=120s -v -cover ./...

test-in-docker: checkdockerfile
	@ln -s scripts/dockerfiles/Dockerfile.test Dockerfile
	docker build -t "amazon/amazon-ecs-agent-test:make" .
	# Privileged needed for docker-in-docker so integ tests pass
	docker run -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-agent" --privileged "amazon/amazon-ecs-agent-test:make"
	@rm -f Dockerfile

netkitten:
	cd misc/netkitten; $(MAKE) $(MFLAGS)

get-deps:
	go get github.com/tools/godep
	go get golang.org/x/tools/cover

clean:
	rm -f misc/certs/ca-certificates.crt &> /dev/null
	rm -f out/amazon-ecs-agent &> /dev/null
	rm -rf agent/Godeps/_workspace/pkg/
	cd misc/netkitten; $(MAKE) $(MFLAGS) clean
