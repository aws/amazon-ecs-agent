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

.PHONY: all gobuild static docker release certs test clean netkitten test-registry run-functional-tests gremlin gogenerate

all: docker

# Dynamic go build; useful in that it does not have -a so it won't recompile
# everything every time
gobuild:
	./scripts/build false

# Basic go build
static:
	./scripts/build

# 'golang-base' builds a Go binary patched for CVE-2015-5739, CVE-2015-5740, and CVE-2015-5741
golang-base:
	@docker build -f scripts/dockerfiles/Dockerfile.golang -t "amazon/amazon-ecs-agent-build:golang" .

# 'build-in-docker' builds the agent within a dockerfile and saves it to the ./out
# directory
build-in-docker: golang-base
	@docker build -f scripts/dockerfiles/Dockerfile.build -t "amazon/amazon-ecs-agent-build:make" .
	@docker run --net=none -v "$(shell pwd)/out:/out" -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-agent" "amazon/amazon-ecs-agent-build:make"

# 'docker' builds the agent dockerfile from the current sourcecode tree, dirty
# or not
docker: certs build-in-docker
	@cd scripts && ./create-amazon-ecs-scratch
	@docker build -f scripts/dockerfiles/Dockerfile.release -t "amazon/amazon-ecs-agent:make" .
	@echo "Built Docker image \"amazon/amazon-ecs-agent:make\""

# 'docker-release' builds the agent from a clean snapshot of the git repo in
# 'RELEASE' mode
docker-release: golang-base
	@docker build -f scripts/dockerfiles/Dockerfile.cleanbuild -t "amazon/amazon-ecs-agent-cleanbuild:make" .
	@docker run --net=none -v "$(shell pwd)/out:/out" -v "$(shell pwd):/src/amazon-ecs-agent" "amazon/amazon-ecs-agent-cleanbuild:make"

# Release packages our agent into a "scratch" based dockerfile
release: certs docker-release
	@./scripts/create-amazon-ecs-scratch
	@docker build -f scripts/dockerfiles/Dockerfile.release -t "amazon/amazon-ecs-agent:latest" .
	@echo "Built Docker image \"amazon/amazon-ecs-agent:latest\""

gogenerate:
	./scripts/gogenerate

# We need to bundle certificates with our scratch-based container
certs: misc/certs/ca-certificates.crt
misc/certs/ca-certificates.crt:
	docker build -t "amazon/amazon-ecs-agent-cert-source:make" misc/certs/
	docker run "amazon/amazon-ecs-agent-cert-source:make" cat /etc/ssl/certs/ca-certificates.crt > misc/certs/ca-certificates.crt

short-test:
	. ./scripts/shared_env && go test -short -timeout=25s ./agent/...

# Run our 'test' registry needed for integ and functional tests
test-registry: netkitten volumes-test squid
	@./scripts/setup-test-registry

test: test-registry gremlin
	. ./scripts/shared_env && go test -timeout=120s -v -cover ./...

test-in-docker:
	docker build -f scripts/dockerfiles/Dockerfile.test -t "amazon/amazon-ecs-agent-test:make" .
	# Privileged needed for docker-in-docker so integ tests pass
	docker run -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-agent" --privileged "amazon/amazon-ecs-agent-test:make"

run-functional-tests: test-registry
	. ./scripts/shared_env && go test -tags functional -timeout=30m -v ./agent/functional_tests/...

netkitten:
	cd misc/netkitten; $(MAKE) $(MFLAGS)

volumes-test:
	cd misc/volumes-test; $(MAKE) $(MFLAGS)

# TODO, replace this with a build on dockerhub or a mechanism for the
# functional tests themselves to build this
.PHONY: squid
squid:
	cd misc/squid; $(MAKE) $(MFLAGS)

gremlin:
	cd misc/gremlin; $(MAKE) $(MFLAGS)

get-deps:
	go get github.com/tools/godep
	go get golang.org/x/tools/cmd/cover
	go get github.com/golang/mock/mockgen
	go get golang.org/x/tools/cmd/goimports


clean:
	rm -f misc/certs/ca-certificates.crt &> /dev/null
	rm -f out/amazon-ecs-agent &> /dev/null
	rm -rf agent/Godeps/_workspace/pkg/
	cd misc/netkitten; $(MAKE) $(MFLAGS) clean
	cd misc/volumes-test; $(MAKE) $(MFLAGS) clean
	cd misc/gremlin; $(MAKE) $(MFLAGS) clean
