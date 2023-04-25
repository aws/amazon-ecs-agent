# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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
# default value of TARGET_OS
TARGET_OS=linux

.PHONY: all gobuild static xplatform-build docker release certs test clean netkitten test-registry benchmark-test gogenerate run-integ-tests pause-container get-cni-sources cni-plugins test-artifacts release-agent release-agent-internal
BUILD_PLATFORM:=$(shell uname -m)

ifeq (${BUILD_PLATFORM},aarch64)
	GOARCH=arm64
else
	GOARCH=amd64
endif

ifeq (${TARGET_OS},windows)
	GO_VERSION=$(shell cat ./GO_VERSION_WINDOWS)
else
	GO_VERSION=$(shell cat ./GO_VERSION)
endif

export GO111MODULE=auto

all: docker

# Dynamic go build; useful in that it does not have -a so it won't recompile
# everything every time
gobuild:
	./scripts/build false

gobuild-init-deb:
	./scripts/gobuild.sh debian

# create output directories
.out-stamp:
	mkdir -p ./out/test-artifacts ./out/cni-plugins ./out/amazon-ecs-cni-plugins ./out/amazon-vpc-cni-plugins
	touch .out-stamp

# Basic go build
static:
	./scripts/build true "" true true

# Cross-platform build target for static checks
xplatform-build:
	GOOS=linux GOARCH=arm64 ./scripts/build true "" false
	GOOS=windows GOARCH=amd64 ./scripts/build true "" false
	# Agent and its dependencies on Go 1.18.x are not compatible with Mac (Darwin).
	# Mac is not a supported target platform for Agent, so commenting out
	# cross-platform build step for Mac temporarily.
	# GOOS=darwin GOARCH=amd64 ./scripts/build true "" false

BUILDER_IMAGE="amazon/amazon-ecs-agent-build:make"
.builder-image-stamp: scripts/dockerfiles/Dockerfile.build
	@docker build --build-arg GO_VERSION=$(GO_VERSION) -f scripts/dockerfiles/Dockerfile.build -t $(BUILDER_IMAGE) .
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

ifeq (${TARGET_OS},windows)
    BUILD="cleanbuild-${TARGET_OS}"
else
    BUILD=cleanbuild
endif

# 'docker-release' builds the agent from a clean snapshot of the git repo in
# 'RELEASE' mode
# TODO: make this idempotent
docker-release: pause-container-release cni-plugins .out-stamp
	@docker build --build-arg GO_VERSION=${GO_VERSION} -f scripts/dockerfiles/Dockerfile.cleanbuild -t "amazon/amazon-ecs-agent-${BUILD}:make" .
	@docker run --net=none \
		--env TARGET_OS="${TARGET_OS}" \
		--env GO111MODULE=auto \
		--env LDFLAGS="-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerTag=$(PAUSE_CONTAINER_TAG) \
			-X github.com/aws/amazon-ecs-agent/agent/config.DefaultPauseContainerImageName=$(PAUSE_CONTAINER_IMAGE)" \
		--user "$(USERID)" \
		--volume "$(PWD)/out:/out" \
		--volume "$(PWD):/src/amazon-ecs-agent" \
		--rm \
		"amazon/amazon-ecs-agent-${BUILD}:make"

# Legacy target : Release packages our agent into a "scratch" based dockerfile
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
	PATH=$(PATH):$(shell pwd)/scripts go generate -x ./agent/... ./ecs-agent/...
	$(MAKE) goimports

gogenerate-init:
	PATH=$(PATH):$(shell pwd)/scripts go generate -x ./ecs-init/...
	$(MAKE) goimports

# 'go' may not be on the $PATH for sudo tests
GO_EXECUTABLE=$(shell command -v go 2> /dev/null)

# VERBOSE includes the options that make the test opt loud
VERBOSE=-v -cover

# -count=1 runs the test without using the build cache.  The build cache can
# provide false positives when running integ tests, so we err on the side of
# caution. See `go help test`
# unit tests include the coverage profile
GOTEST=${GO_EXECUTABLE} test -count=1

# -race sometimes causes compile issues on Arm
ifneq (${BUILD_PLATFORM},aarch64)
	GOTEST += -race
endif

test:
	cd agent && GO111MODULE=on ${GOTEST} ${VERBOSE} -tags unit -mod vendor -coverprofile ../cover.out -timeout=120s ./... && cd ..
	go tool cover -func cover.out > coverprofile.out
	cd ecs-agent && GO111MODULE=on ${GOTEST} ${VERBOSE} -tags unit -mod vendor -coverprofile ../cover.out -timeout=120s ./... && cd ..
	go tool cover -func cover.out > coverprofile-ecs-agent.out

test-init:
	go test -count=1 -short -v -coverprofile cover.out ./ecs-init/...
	go tool cover -func cover.out > coverprofile-init.out

test-silent:
	cd agent && GO111MODULE=on ${GOTEST} -tags unit -mod vendor -coverprofile ../cover.out -timeout=120s ./... && cd ..
	go tool cover -func cover.out > coverprofile.out
	cd ecs-agent && GO111MODULE=on ${GOTEST} -tags unit -mod vendor -coverprofile ../cover.out -timeout=120s ./... && cd ..
	go tool cover -func cover.out > coverprofile-ecs-agent.out

.PHONY: analyze-cover-profile
analyze-cover-profile: coverprofile.out coverprofile-ecs-agent.out
	./scripts/analyze-cover-profile coverprofile.out
	./scripts/analyze-cover-profile coverprofile-ecs-agent.out

.PHONY: analyze-cover-profile-init
analyze-cover-profile-init: coverprofile-init.out
	./scripts/analyze-cover-profile coverprofile-init.out

run-integ-tests: test-registry gremlin container-health-check-image run-sudo-tests
	ECS_LOGLEVEL=debug ${GOTEST} -tags integration -timeout=30m ./agent/...
	ECS_LOGLEVEL=debug ${GOTEST} -tags integration -timeout=30m ./ecs-agent/...

run-sudo-tests:
	sudo -E ${GOTEST} -tags sudo -timeout=10m ./agent/...

run-sudo-unit-tests:
	sudo -E ${GOTEST} -tags 'sudo_unit' -timeout=60s ./agent/...

benchmark-test:
	go test -run=XX -bench=. ./agent/...

.PHONY: build-image-for-ecr upload-images replicate-images

build-image-for-ecr: netkitten volumes-test image-cleanup-test-images fluentd exec-command-agent-test

upload-images: build-image-for-ecr
	@./scripts/upload-images $(STANDARD_REGION) $(STANDARD_REPOSITORY)

replicate-images: build-image-for-ecr
	@./scripts/upload-images $(REPLICATE_REGION) $(REPLICATE_REPOSITORY)

PAUSE_CONTAINER_IMAGE = "amazon/amazon-ecs-pause"
PAUSE_CONTAINER_TAG = "0.1.0"
PAUSE_CONTAINER_TARBALL = "amazon-ecs-pause.tar"

pause-container: .out-stamp
	$(MAKE) -C misc/pause-container $(MFLAGS)

pause-container-release: pause-container
	@docker save ${PAUSE_CONTAINER_IMAGE}:${PAUSE_CONTAINER_TAG} > "$(PWD)/out/${PAUSE_CONTAINER_TARBALL}"

# Variable to override cni repository location
ECS_CNI_REPOSITORY_SRC_DIR=$(PWD)/amazon-ecs-cni-plugins
VPC_CNI_REPOSITORY_SRC_DIR=$(PWD)/amazon-vpc-cni-plugins

# Variable to store the platform specific dockerfile for vpc-cni-plugins
VPC_CNI_REPOSITORY_DOCKER_FILE=scripts/dockerfiles/Dockerfile.buildVPCCNIPlugins
ifeq (${TARGET_OS}, windows)
    VPC_CNI_REPOSITORY_DOCKER_FILE=scripts/dockerfiles/Dockerfile.buildWindowsVPCCNIPlugins
endif

get-cni-sources:
	git submodule update --init --recursive

build-ecs-cni-plugins:
	@docker build --build-arg GO_VERSION=$(GO_VERSION) -f scripts/dockerfiles/Dockerfile.buildECSCNIPlugins -t "amazon/amazon-ecs-build-ecs-cni-plugins:make" .
	docker run --rm --net=none \
		-e GO111MODULE=auto \
		-e GIT_SHORT_HASH=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git rev-parse --short=8 HEAD) \
		-e GIT_PORCELAIN=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git status --porcelain 2> /dev/null | wc -l | sed 's/^ *//') \
		-u "$(USERID)" \
		-v "$(PWD)/out/amazon-ecs-cni-plugins:/go/src/github.com/aws/amazon-ecs-cni-plugins/bin/plugins" \
		-v "$(ECS_CNI_REPOSITORY_SRC_DIR):/go/src/github.com/aws/amazon-ecs-cni-plugins" \
		"amazon/amazon-ecs-build-ecs-cni-plugins:make"
	@echo "Built amazon-ecs-cni-plugins successfully."

build-vpc-cni-plugins:
	@docker build --build-arg GOARCH=$(GOARCH) --build-arg GO_VERSION=$(GO_VERSION) -f $(VPC_CNI_REPOSITORY_DOCKER_FILE) -t "amazon/amazon-ecs-build-vpc-cni-plugins:make" .
	docker run --rm --net=none \
		-e GO111MODULE=off \
		-e GIT_SHORT_HASH=$(shell cd $(VPC_CNI_REPOSITORY_SRC_DIR) && git rev-parse --short=8 HEAD) \
		-e GIT_TAG=$(shell cd $(VPC_CNI_REPOSITORY_SRC_DIR) && git describe --tags --always --dirty) \
		-u "$(USERID)" \
		-v "$(PWD)/out/amazon-vpc-cni-plugins:/go/src/github.com/aws/amazon-vpc-cni-plugins/build/${TARGET_OS}_$(GOARCH)" \
		-v "$(VPC_CNI_REPOSITORY_SRC_DIR):/go/src/github.com/aws/amazon-vpc-cni-plugins" \
		"amazon/amazon-ecs-build-vpc-cni-plugins:make"
	@echo "Built amazon-vpc-cni-plugins successfully."

cni-plugins: get-cni-sources .out-stamp build-ecs-cni-plugins build-vpc-cni-plugins
	mv $(PWD)/out/amazon-ecs-cni-plugins/* $(PWD)/out/cni-plugins
	mv $(PWD)/out/amazon-vpc-cni-plugins/* $(PWD)/out/cni-plugins
	@echo "Built all cni plugins successfully."

# dockerfree build process will build the agent container image from scratch
# requires glibc-static -- we precompile amd/arm to the misc/pause-container/pause-image-tar-files/ directory
dockerfree-pause:
	./scripts/build-pause

dockerfree-certs:
	./scripts/get-host-certs

dockerfree-cni-plugins:
	./scripts/build-cni-plugins

# see dockerfree-pause above: assumes that the pre-compiled pause container tar exists
# builds agent image and saves on disk, assumes cni plugins have been pulled
release-agent-internal: dockerfree-certs dockerfree-cni-plugins static
	./scripts/build-agent-image

# Default Agent target to build. Pulls cni plugins, builds agent image and save it to disk
release-agent: get-cni-sources
	$(MAKE) release-agent-internal

# Legacy target used for building agent artifacts for functional tests
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

# Run our 'test' registry needed for integ tests
test-registry: netkitten volumes-test pause-container image-cleanup-test-images fluentd exec-command-agent-test

exec-command-agent-test:
	$(MAKE) -C misc/exec-command-agent-test $(MFLAGS)

	@./scripts/setup-test-registry

.PHONY: fluentd gremlin image-cleanup-test-images

gremlin:
	$(MAKE) -C misc/gremlin $(MFLAGS)

fluentd:
	$(MAKE) -C misc/fluentd $(MFLAGS)

image-cleanup-test-images:
	$(MAKE) -C misc/image-cleanup-test-images $(MFLAGS)

container-health-check-image:
	$(MAKE) -C misc/container-health $(MFLAGS)

# all .go files in the agent, excluding vendor/, model/ and testutils/ directories, and all *_test.go and *_mocks.go files
GOFILES:=$(shell go list -f '{{$$p := .}}{{range $$f := .GoFiles}}{{$$p.Dir}}/{{$$f}} {{end}}' ./agent/... ./ecs-agent/... \
		| grep -v /testutils/ | grep -v _test\.go$ | grep -v _mocks\.go$ | grep -v /model)

.PHONY: gocyclo
gocyclo:
	# Run gocyclo over all .go files
	gocyclo -over 17 ${GOFILES}

# same as gofiles above, but without the `-f`
.PHONY: govet
govet:
	go vet $(shell go list ./agent/... ./ecs-agent/... | grep -v /testutils/ | grep -v _test\.go$ | grep -v /mocks | grep -v /model)

GOFMTFILES:=$(shell find ./agent ./ecs-agent -not -path './agent/vendor/*' -not -path './ecs-agent/vendor/*' -type f -iregex '.*\.go')

.PHONY: importcheck
importcheck:
	$(eval DIFFS:=$(shell goimports -l $(GOFMTFILES)))
	@if [ -n "$(DIFFS)" ]; then echo "Files incorrectly formatted. Fix formatting by running goimports:"; echo "$(DIFFS)"; exit 1; fi

.PHONY: gogenerate-check
gogenerate-check: gogenerate
	# check that gogenerate does not generate a diff.
	git diff --exit-code

.PHONY: gogenerate-check-init
gogenerate-check-init: gogenerate-init
	# check that gogenerate does not generate a diff.
	git diff --exit-code

.PHONY: static-check
static-check: gocyclo govet importcheck gogenerate-check
	# use default checks of staticcheck tool, except style checks (-ST*) and depracation checks (-SA1019)
	# depracation checks have been left out for now; removing their warnings requires error handling for newer suggested APIs, changes in function signatures and their usages.
	# https://github.com/dominikh/go-tools/tree/master/cmd/staticcheck
	staticcheck -tests=false -checks "inherit,-ST*,-SA1019,-SA9002,-SA4006" ./agent/... ./ecs-agent/...

.PHONY: static-check-init
static-check-init: gocyclo govet importcheck gogenerate-check-init
	# use default checks of staticcheck tool, except style checks (-ST*)
	# https://github.com/dominikh/go-tools/tree/master/cmd/staticcheck
	staticcheck -tests=false -checks "inherit,-ST*" ./ecs-init/...

.PHONY: goimports
goimports:
	goimports -w $(GOFMTFILES)

GOPATH=$(shell go env GOPATH)

install-golang:
	./scripts/install-golang.sh

.get-deps-stamp:
	go install github.com/golang/mock/mockgen@v1.6.0
	go install golang.org/x/tools/cmd/goimports@v0.2.0
	GO111MODULE=on go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
	GO111MODULE=on go install honnef.co/go/tools/cmd/staticcheck@v0.3.2
	touch .get-deps-stamp

get-deps: .get-deps-stamp

get-deps-init:
	go install github.com/golang/mock/mockgen@v1.6.0
	go install golang.org/x/tools/cmd/goimports@v0.2.0
	GO111MODULE=on go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
	GO111MODULE=on go install honnef.co/go/tools/cmd/staticcheck@v0.3.2

amazon-linux-sources.tgz:
	./scripts/update-version.sh
	cp packaging/amazon-linux-ami-integrated/ecs-agent.spec ecs-agent.spec
	cp packaging/amazon-linux-ami-integrated/ecs.conf ecs.conf
	cp packaging/amazon-linux-ami-integrated/ecs.service ecs.service
	cp packaging/amazon-linux-ami-integrated/amazon-ecs-volume-plugin.conf amazon-ecs-volume-plugin.conf
	cp packaging/amazon-linux-ami-integrated/amazon-ecs-volume-plugin.service amazon-ecs-volume-plugin.service
	cp packaging/amazon-linux-ami-integrated/amazon-ecs-volume-plugin.socket amazon-ecs-volume-plugin.socket
	tar -czf ./sources.tgz ecs-init scripts misc agent amazon-ecs-cni-plugins amazon-vpc-cni-plugins agent-container Makefile VERSION RELEASE_COMMIT

.amazon-linux-rpm-integrated-done: amazon-linux-sources.tgz
	test -e SOURCES || ln -s . SOURCES
	rpmbuild --define "%_topdir $(PWD)" -bb ecs-agent.spec
	find RPMS/ -type f -exec cp {} . \;
	touch .amazon-linux-rpm-integrated-done

amazon-linux-rpm-integrated: .amazon-linux-rpm-integrated-done

.generic-rpm-integrated-done: get-cni-sources
	./scripts/update-version.sh
	cp packaging/generic-rpm-integrated/amazon-ecs-init.spec amazon-ecs-init.spec
	cp packaging/generic-rpm-integrated/ecs.service ecs.service
	cp packaging/generic-rpm-integrated/amazon-ecs-volume-plugin.service amazon-ecs-volume-plugin.service
	cp packaging/generic-rpm-integrated/amazon-ecs-volume-plugin.socket amazon-ecs-volume-plugin.socket
	tar -czf ./sources.tgz ecs-init scripts misc agent amazon-ecs-cni-plugins amazon-vpc-cni-plugins agent-container Makefile VERSION GO_VERSION
	test -e SOURCES || ln -s . SOURCES
	rpmbuild --define "%_topdir $(PWD)" -bb amazon-ecs-init.spec
	find RPMS/ -type f -exec cp {} . \;
	touch .generic-rpm-integrated-done

# Build init rpm
generic-rpm-integrated: .generic-rpm-integrated-done

VERSION = $(shell cat ecs-init/ECSVERSION)

.generic-deb-integrated-done: get-cni-sources
	./scripts/update-version.sh
	mkdir -p BUILDROOT
	tar -czf ./amazon-ecs-init_${VERSION}.orig.tar.gz ecs-init scripts README.md
	cp -r packaging/generic-deb-integrated/debian Makefile ecs-init scripts misc agent agent-container amazon-ecs-cni-plugins amazon-vpc-cni-plugins README.md VERSION GO_VERSION BUILDROOT
	cd BUILDROOT && dpkg-buildpackage -uc -b
	touch .generic-deb-integrated-done

# Build init deb
generic-deb-integrated: .generic-deb-integrated-done

ARCH:=$(shell uname -m)
ifeq (${ARCH},x86_64)
	AGENT_FILENAME=ecs-agent-v${VERSION}.tar
else ifeq (${ARCH},aarch64)
	AGENT_FILENAME=ecs-agent-arm64-v${VERSION}.tar
# osx M1 instances
else ifeq (${ARCH},arm64)
	AGENT_FILENAME=ecs-agent-arm64-v${VERSION}.tar
endif

clean:
	-rm -f misc/certs/host-certs.crt &> /dev/null
	-rm -rf misc/pause-container/image/
	-rm -rf misc/pause-container/rootfs/
	-rm -rf misc/plugins/
	-rm -rf out/
	-rm -rf rootfs/
	-$(MAKE) -C $(ECS_CNI_REPOSITORY_SRC_DIR) clean
	-rm -f .get-deps-stamp
	-rm -f .builder-image-stamp
	-rm -f .out-stamp
	-rm -f ecs-agent.spec
	-rm -rf $(PWD)/bin
	-rm -rf cover.out
	-rm -rf coverprofile.out
	-rm -rf coverprofile-init.out
	-rm -rf coverprofile-ecs-agent.out
	# ecs-init & rpm cleanup
	-rm -f ecs-init.spec
	-rm -f amazon-ecs-init.spec
	-rm -f ecs.conf
	-rm -f ecs.service
	-rm -f amazon-ecs-volume-plugin.conf
	-rm -f amazon-ecs-volume-plugin.service
	-rm -f amazon-ecs-volume-plugin.socket
	-rm -rf ./bin
	-rm -f ./sources.tgz
	-rm -f ./amazon-ecs-init
	-rm -f ./ecs-init/ecs-init
	-rm -f ./amazon-ecs-init-*.rpm
	-rm -f ./ecs-agent-*.tar
	-rm -f ./ecs-init-*.src.rpm
	-rm -rf ./ecs-init-*
	-rm -rf ./BUILDROOT BUILD RPMS SRPMS SOURCES SPECS
	-rm -rf ./x86_64
	-rm -f ./amazon-ecs-init_${VERSION}*
	-rm -f .srpm-done .rpm-done .generic-rpm-done .generic-deb-integrated-done
	-rm -f .deb-done
	-rm -f .amazon-linux-rpm-integrated-done
	-rm -f .generic-rpm-integrated-done
	-rm -f amazon-ecs-volume-plugin

clean-all: clean
	# for our dockerfree builds, we likely don't have docker
	# ensure docker is running and we can talk to it, abort if not:
	docker ps > /dev/null
	-docker rmi $(BUILDER_IMAGE) "amazon/amazon-ecs-agent-cleanbuild:make"
	-docker rmi $(BUILDER_IMAGE) "amazon/amazon-ecs-agent-cleanbuild-windows:make"
	-$(MAKE) -C misc/netkitten $(MFLAGS) clean
	-$(MAKE) -C misc/volumes-test $(MFLAGS) clean
	-$(MAKE) -C misc/exec-command-agent-test $(MFLAGS) clean
	-$(MAKE) -C misc/gremlin $(MFLAGS) clean
	-$(MAKE) -C misc/image-cleanup-test-images $(MFLAGS) clean
	-$(MAKE) -C misc/container-health $(MFLAGS) clean
