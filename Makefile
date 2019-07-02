# Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

USERID=$(shell id -u)

.PHONY: all gobuild static xplatform-build docker release certs test clean netkitten test-registry namespace-tests run-functional-tests benchmark-test gogenerate run-integ-tests pause-container get-cni-sources cni-plugins test-artifacts
BUILD_PLATFORM:=$(shell uname -m)

ifeq (${BUILD_PLATFORM},aarch64)
	GOARCH=arm64
else
	GOARCH=amd64
endif

all: docker

# Dynamic go build; useful in that it does not have -a so it won't recompile
# everything every time
gobuild:
	./scripts/build false


# create output directories
.out-stamp:
	mkdir -p ./out/test-artifacts ./out/cni-plugins ./out/amazon-ecs-cni-plugins ./out/amazon-vpc-cni-plugins
	touch .out-stamp

# Basic go build
static:
	./scripts/build

# Cross-platform build target for travis
xplatform-build:
	GOOS=linux GOARCH=arm64 ./scripts/build true "" false
	GOOS=windows GOARCH=amd64 ./scripts/build true "" false
	GOOS=darwin GOARCH=amd64 ./scripts/build true "" false

BUILDER_IMAGE="amazon/amazon-ecs-agent-build:make"
.builder-image-stamp: scripts/dockerfiles/Dockerfile.build
	@docker build -f scripts/dockerfiles/Dockerfile.build -t $(BUILDER_IMAGE) .
	touch .builder-image-stamp

# 'build-in-docker' builds the agent within a dockerfile and saves it to the ./out
# directory
# TODO: make this idempotent
build-in-docker: .builder-image-stamp .out-stamp
	@docker run --net=none \
		--env TARGET_OS="${TARGET_OS}" \
		--env LDFLAGS="-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerTag=$(PAUSE_CONTAINER_TAG) \
			-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerImageName=$(PAUSE_CONTAINER_IMAGE)" \
		--volume "$(PWD)/out:/out" \
		--volume "$(PWD):/go/src/github.com/aws/amazon-ecs-agent" \
		--user "$(USERID)" \
		--rm \
		$(BUILDER_IMAGE)

# 'docker' builds the agent dockerfile from the current sourcecode tree, dirty
# or not
docker: certs build-in-docker pause-container-release cni-plugins .out-stamp
	@cd scripts && ./create-amazon-ecs-scratch
	@docker build -f scripts/dockerfiles/Dockerfile.release -t "amazon/amazon-ecs-agent:make" .
	@echo "Built Docker image \"amazon/amazon-ecs-agent:make\""

# 'docker-release' builds the agent from a clean snapshot of the git repo in
# 'RELEASE' mode
# TODO: make this idempotent
docker-release: pause-container-release cni-plugins .out-stamp
	@docker build -f scripts/dockerfiles/Dockerfile.cleanbuild -t "amazon/amazon-ecs-agent-cleanbuild:make" .
	@docker run --net=none \
		--env TARGET_OS="${TARGET_OS}" \
		--env LDFLAGS="-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerTag=$(PAUSE_CONTAINER_TAG) \
			-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerImageName=$(PAUSE_CONTAINER_IMAGE)" \
		--user "$(USERID)" \
		--volume "$(PWD)/out:/out" \
		--volume "$(PWD):/src/amazon-ecs-agent" \
		--rm \
		"amazon/amazon-ecs-agent-cleanbuild:make"

# Release packages our agent into a "scratch" based dockerfile
release: certs docker-release
	@./scripts/create-amazon-ecs-scratch
	@docker build -f scripts/dockerfiles/Dockerfile.release -t "amazon/amazon-ecs-agent:latest" .
	@echo "Built Docker image \"amazon/amazon-ecs-agent:latest\""

# We need to bundle certificates with our scratch-based container
certs: misc/certs/ca-certificates.crt
misc/certs/ca-certificates.crt:
	docker build -t "amazon/amazon-ecs-agent-cert-source:make" misc/certs/
	docker run "amazon/amazon-ecs-agent-cert-source:make" cat /etc/ssl/certs/ca-certificates.crt > misc/certs/ca-certificates.crt

gogenerate:
	go generate -x ./agent/...
	$(MAKE) goimports

# 'go' may not be on the $PATH for sudo tests
GO_EXECUTABLE=$(shell command -v go 2> /dev/null)

# VERBOSE includes the options that make the test opt loud
VERBOSE=-v -cover

# -count=1 runs the test without using the build cache.  The build cache can
# provide false positives when running integ tests, so we err on the side of
# caution. See `go help test`
GOTEST=${GO_EXECUTABLE} test -count=1 ${VERBOSE}

# -race sometimes causes compile issues on Arm
ifneq (${BUILD_PLATFORM},aarch64)
	GOTEST += -race
endif

test:
	${GOTEST} -tags unit -timeout=30s ./agent/...

test-silent:
	$(eval undefine VERBOSE)
	${GOTEST} -tags unit -timeout=30s ./agent/...

run-integ-tests: test-registry gremlin container-health-check-image run-sudo-tests
	ECS_LOGLEVEL=debug ${GOTEST} -tags integration -timeout=25m ./agent/...

run-sudo-tests:
	sudo -E ${GOTEST} -tags sudo -timeout=10m ./agent/...

run-functional-tests: testnnp test-registry ecr-execution-role-image telemetry-test-image storage-stats-test-image
	${GOTEST} -tags functional -timeout=60m ./agent/functional_tests/...

benchmark-test:
	go test -run=XX -bench=. ./agent/...


.PHONY: build-image-for-ecr ecr-execution-role-image-for-upload upload-images replicate-images

build-image-for-ecr: netkitten volumes-test squid awscli image-cleanup-test-images fluentd taskmetadata-validator \
						testnnp container-health-check-image telemetry-test-image storage-stats-test-image ecr-execution-role-image-for-upload

ecr-execution-role-image-for-upload:
	$(MAKE) -C misc/ecr-execution-role-upload $(MFLAGS)

upload-images: build-image-for-ecr
	@./scripts/upload-images $(STANDARD_REGION) $(STANDARD_REPOSITORY)

replicate-images: build-image-for-ecr
	@./scripts/upload-images $(REPLICATE_REGION) $(REPLICATE_REPOSITORY)

PAUSE_CONTAINER_IMAGE = "amazon/amazon-ecs-pause"
PAUSE_CONTAINER_TAG = "0.1.0"
PAUSE_CONTAINER_TARBALL = "amazon-ecs-pause.tar"

pause-container: .out-stamp
	@docker build -f scripts/dockerfiles/Dockerfile.buildPause -t "amazon/amazon-ecs-build-pause-bin:make" .
	@docker run --net=none \
		-u "$(USERID)" \
		-v "$(PWD)/misc/pause-container:/out" \
		-v "$(PWD)/misc/pause-container/buildPause:/usr/src/buildPause" \
		"amazon/amazon-ecs-build-pause-bin:make"

	$(MAKE) -C misc/pause-container $(MFLAGS)
	@docker rmi -f "amazon/amazon-ecs-build-pause-bin:make"

pause-container-release: pause-container
	@docker save ${PAUSE_CONTAINER_IMAGE}:${PAUSE_CONTAINER_TAG} > "$(PWD)/out/${PAUSE_CONTAINER_TARBALL}"

# Variable to determine branch/tag of amazon-ecs-cni-plugins
ECS_CNI_REPOSITORY_REVISION=master

# Variable to override cni repository location
ECS_CNI_REPOSITORY_SRC_DIR=$(PWD)/amazon-ecs-cni-plugins
VPC_CNI_REPOSITORY_SRC_DIR=$(PWD)/amazon-vpc-cni-plugins

get-cni-sources:
	git submodule update --init --recursive --remote

build-ecs-cni-plugins:
	@docker build -f scripts/dockerfiles/Dockerfile.buildECSCNIPlugins -t "amazon/amazon-ecs-build-ecs-cni-plugins:make" .
	docker run --rm --net=none \
		-e GIT_SHORT_HASH=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git rev-parse --short=8 HEAD) \
		-e GIT_PORCELAIN=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git status --porcelain 2> /dev/null | wc -l | sed 's/^ *//') \
		-u "$(USERID)" \
		-v "$(PWD)/out/amazon-ecs-cni-plugins:/go/src/github.com/aws/amazon-ecs-cni-plugins/bin/plugins" \
		-v "$(ECS_CNI_REPOSITORY_SRC_DIR):/go/src/github.com/aws/amazon-ecs-cni-plugins" \
		"amazon/amazon-ecs-build-ecs-cni-plugins:make"
	@echo "Built amazon-ecs-cni-plugins successfully."

build-vpc-cni-plugins:
	@docker build --build-arg GOARCH=$(GOARCH) -f scripts/dockerfiles/Dockerfile.buildVPCCNIPlugins -t "amazon/amazon-ecs-build-vpc-cni-plugins:make" .
	docker run --rm --net=none \
		-e GIT_SHORT_HASH=$(shell cd $(VPC_CNI_REPOSITORY_SRC_DIR) && git rev-parse --short=8 HEAD) \
		-u "$(USERID)" \
		-v "$(PWD)/out/amazon-vpc-cni-plugins:/go/src/github.com/aws/amazon-vpc-cni-plugins/build/linux_$(GOARCH)" \
		-v "$(VPC_CNI_REPOSITORY_SRC_DIR):/go/src/github.com/aws/amazon-vpc-cni-plugins" \
		"amazon/amazon-ecs-build-vpc-cni-plugins:make"
	@echo "Built amazon-vpc-cni-plugins successfully."

cni-plugins: get-cni-sources .out-stamp build-ecs-cni-plugins build-vpc-cni-plugins
	mv $(PWD)/out/amazon-ecs-cni-plugins/* $(PWD)/out/cni-plugins
	mv $(PWD)/out/amazon-vpc-cni-plugins/* $(PWD)/out/cni-plugins
	@echo "Built all cni plugins successfully."


.PHONY: codebuild
codebuild: .out-stamp
	$(MAKE) release TARGET_OS="linux"
	TARGET_OS="linux" ./scripts/local-save
	$(MAKE) docker-release TARGET_OS="windows"
	TARGET_OS="windows" ./scripts/local-save

netkitten:
	$(MAKE) -C misc/netkitten $(MFLAGS)

volumes-test:
	$(MAKE) -C misc/volumes-test $(MFLAGS)

namespace-tests:
	@docker build -f scripts/dockerfiles/Dockerfile.buildNamespaceTests -t "amazon/amazon-ecs-namespace-tests:make" .
	@docker run --net=none \
		-u "$(USERID)" \
		-v "$(PWD)/misc/namespace-tests:/out" \
		-v "$(PWD)/misc/namespace-tests/buildContainer:/usr/src/buildContainer" \
		"amazon/amazon-ecs-namespace-tests:make"

	$(MAKE) -C misc/namespace-tests $(MFLAGS)
	@docker rmi -f "amazon/amazon-ecs-namespace-tests:make"

# Run our 'test' registry needed for integ and functional tests
test-registry: netkitten volumes-test namespace-tests pause-container squid awscli image-cleanup-test-images fluentd \
				agent-introspection-validator taskmetadata-validator v3-task-endpoint-validator \
				container-metadata-file-validator elastic-inference-validator appmesh-plugin-validator \
				eni-trunking-validator
	@./scripts/setup-test-registry


# TODO, replace this with a build on dockerhub or a mechanism for the
# functional tests themselves to build this
.PHONY: squid awscli fluentd gremlin agent-introspection-validator taskmetadata-validator v3-task-endpoint-validator container-metadata-file-validator elastic-inference-validator image-cleanup-test-images ecr-execution-role-image container-health-check-image telemetry-test-image storage-stats-test-image nfs-server-image

squid:
	$(MAKE) -C misc/squid $(MFLAGS)

gremlin:
	$(MAKE) -C misc/gremlin $(MFLAGS)

awscli:
	$(MAKE) -C misc/awscli $(MFLAGS)

fluentd:
	$(MAKE) -C misc/fluentd $(MFLAGS)

testnnp:
	$(MAKE) -C misc/testnnp $(MFLAGS)

image-cleanup-test-images:
	$(MAKE) -C misc/image-cleanup-test-images $(MFLAGS)

agent-introspection-validator:
	$(MAKE) -C misc/agent-introspection-validator $(MFLAGS)

taskmetadata-validator:
	$(MAKE) -C misc/taskmetadata-validator $(MFLAGS)

v3-task-endpoint-validator:
	$(MAKE) -C misc/v3-task-endpoint-validator $(MFLAGS)

eni-trunking-validator:
	$(MAKE) -C misc/eni-trunking-validator $(MFLAGS)

container-metadata-file-validator:
	$(MAKE) -C misc/container-metadata-file-validator $(MFLAGS)

elastic-inference-validator:
	$(MAKE) -C misc/elastic-inference-validator $(MFLAGS)

ecr-execution-role-image:
	$(MAKE) -C misc/ecr $(MFLAGS)

telemetry-test-image:
	$(MAKE) -C misc/telemetry $(MFLAGS)

storage-stats-test-image:
	$(MAKE) -C misc/storage-stats $(MFLAGS)

container-health-check-image:
	$(MAKE) -C misc/container-health $(MFLAGS)

appmesh-plugin-validator:
	$(MAKE) -C misc/appmesh-plugin-validator $(MFLAGS)

nfs-server-image:
	cd misc/nfs; $(MAKE) $(MFLAGS)

# all .go files in the agent, excluding vendor/, model/ and testutils/ directories, and all *_test.go and *_mocks.go files
GOFILES:=$(shell go list -f '{{$$p := .}}{{range $$f := .GoFiles}}{{$$p.Dir}}/{{$$f}} {{end}}' ./agent/... \
		| grep -v /testutils/ | grep -v _test\.go$ | grep -v _mocks\.go$ | grep -v /model)

.PHONY: gocyclo
gocyclo:
	# Run gocyclo over all .go files
	gocyclo -over 15 ${GOFILES}

# same as gofiles above, but without the `-f`
.PHONY: govet
govet:
	go vet $(shell go list ./agent/... | grep -v /testutils/ | grep -v _test\.go$ | grep -v /mocks | grep -v /model)

GOFMTSTRING:='{{$$dir := .Dir}}{{range $$f := .GoFiles}}{{$$dir}}/{{$$f}} {{end}}{{range $$f := .IgnoredGoFiles}}{{$$dir}}/{{$$f}} {{end}}'
GOFMTFILES:=$(shell go list -f $(GOFMTSTRING) ./agent/...)
.PHONY: fmtcheck
fmtcheck:
	$(eval DIFFS:=$(shell gofmt -l $(GOFMTFILES)))
	@if [ -n "$(DIFFS)" ]; then echo "Files incorrectly formatted. Fix formatting by running gofmt:"; echo "$(DIFFS)"; exit 1; fi

.PHONY: importcheck
importcheck:
	$(eval DIFFS:=$(shell goimports -l $(GOFMTFILES)))
	@if [ -n "$(DIFFS)" ]; then echo "Files incorrectly formatted. Fix formatting by running goimports:"; echo "$(DIFFS)"; exit 1; fi

.PHONY: static-check
static-check: gocyclo fmtcheck govet importcheck

.PHONY: goimports
goimports:
	goimports -w $(GOFMTFILES)

.PHONY: gofmt
gofmt:
	go fmt ./agent/...

.get-deps-stamp:
	go get golang.org/x/tools/cmd/cover
	go get github.com/golang/mock/mockgen
	go get golang.org/x/tools/cmd/goimports
	go get github.com/fzipp/gocyclo
	touch .get-deps-stamp

get-deps: .get-deps-stamp


PLATFORM:=$(shell uname -s)
ifeq (${PLATFORM},Linux)
		dep_arch=linux-386
	else ifeq (${PLATFORM},Darwin)
		dep_arch=darwin-386
	endif

DEP_VERSION=v0.5.0
.PHONY: get-dep
get-dep: bin/dep

bin/dep:
	mkdir -p ./bin
	curl -L https://github.com/golang/dep/releases/download/$(DEP_VERSION)/dep-${dep_arch} -o ./bin/dep
	chmod +x ./bin/dep

clean:
	# ensure docker is running and we can talk to it, abort if not:
	docker ps > /dev/null
	-docker rmi $(BUILDER_IMAGE) "amazon/amazon-ecs-agent-cleanbuild:make"
	rm -f misc/certs/ca-certificates.crt &> /dev/null
	rm -rf out/
	-$(MAKE) -C $(ECS_CNI_REPOSITORY_SRC_DIR) clean
	-$(MAKE) -C misc/netkitten $(MFLAGS) clean
	-$(MAKE) -C misc/volumes-test $(MFLAGS) clean
	-$(MAKE) -C misc/namespace-tests $(MFLAGS) clean
	-$(MAKE) -C misc/gremlin $(MFLAGS) clean
	-$(MAKE) -C misc/testnnp $(MFLAGS) clean
	-$(MAKE) -C misc/image-cleanup-test-images $(MFLAGS) clean
	-$(MAKE) -C misc/agent-introspection-validator $(MFLAGS) clean
	-$(MAKE) -C misc/taskmetadata-validator $(MFLAGS) clean
	-$(MAKE) -C misc/v3-task-endpoint-validator $(MFLAGS) clean
	-$(MAKE) -C misc/container-metadata-file-validator $(MFLAGS) clean
	-$(MAKE) -C misc/elastic-inference-validator $(MFLAGS) clean
	-$(MAKE) -C misc/container-health $(MFLAGS) clean
	-$(MAKE) -C misc/telemetry $(MFLAGS) clean
	-$(MAKE) -C misc/storage-stats $(MFLAGS) clean
	-$(MAKE) -C misc/appmesh-plugin-validator $(MFLAGS) clean
	-$(MAKE) -C misc/eni-trunking-validator $(MFLAGS) clean
	-$(MAKE) -C misc/nfs $(MFLAGS) clean
	-rm -f .get-deps-stamp
	-rm -f .builder-image-stamp
	-rm -f .out-stamp
	-rm -rf $(PWD)/bin

