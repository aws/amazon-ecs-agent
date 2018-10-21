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
GO_EXECUTABLE=$(shell command -v go 2> /dev/null)

.PHONY: all gobuild static docker release certs test clean netkitten test-registry run-functional-tests benchmark-test gogenerate run-integ-tests pause-container get-cni-sources cni-plugins test-artifacts

all: docker

# Dynamic go build; useful in that it does not have -a so it won't recompile
# everything every time
gobuild:
	./scripts/build false

# create output directories
.out-stamp:
	mkdir -p ./out/test-artifacts ./out/cni-plugins
	touch .out-stamp

# Basic go build
static:
	./scripts/build

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

gogenerate:
	./scripts/gogenerate

# We need to bundle certificates with our scratch-based container
certs: misc/certs/ca-certificates.crt
misc/certs/ca-certificates.crt:
	docker build -t "amazon/amazon-ecs-agent-cert-source:make" misc/certs/
	docker run "amazon/amazon-ecs-agent-cert-source:make" cat /etc/ssl/certs/ca-certificates.crt > misc/certs/ca-certificates.crt

test:
	. ./scripts/shared_env && go test -race -tags unit -timeout=25s -v -cover $(shell go list ./agent/... | grep -v /vendor/)

test-silent:
	. ./scripts/shared_env && go test -timeout=25s -cover $(shell go list ./agent/... | grep -v /vendor/)

benchmark-test:
	. ./scripts/shared_env && go test -run=XX -bench=. $(shell go list ./agent/... | grep -v /vendor/)

define dockerbuild
	docker run \
		--net none \
		--user "$(USERID)" \
		--volume "$(PWD)/out:/out" \
		--volume "$(PWD):/go/src/github.com/aws/amazon-ecs-agent" \
		--rm \
		$(BUILDER_IMAGE) \
		$(1)
endef

define win-cgo-dockerbuild
	docker run --net=none \
		--user "$(USERID)" \
		--volume "$(PWD)/out:/out" \
		--volume "$(PWD):/go/src/github.com/aws/amazon-ecs-agent" \
		--env "GOOS=windows" \
		--env "CGO_ENABLED=1" \
		--env "CC=x86_64-w64-mingw32-gcc" \
		--rm \
		$(BUILDER_IMAGE) \
		$(1)
endef

# TODO: use `go list -f` to target the test files more directly
ALL_GO_FILES = $(shell find . -name "*.go" -print | tr "\n" " ")
GO_INTEG_TEST = go test -race -tags integration -c -o
out/test-artifacts/linux-engine-tests: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call dockerbuild,$(GO_INTEG_TEST) $@ ./agent/engine)

out/test-artifacts/linux-app-tests: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call dockerbuild,$(GO_INTEG_TEST) $@ ./agent/app)

out/test-artifacts/linux-stats-tests: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call dockerbuild,$(GO_INTEG_TEST) $@ ./agent/stats)

out/test-artifacts/windows-engine-tests.exe: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call win-cgo-dockerbuild,$(GO_INTEG_TEST) $@ ./agent/engine)

out/test-artifacts/windows-app-tests.exe: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call win-cgo-dockerbuild,$(GO_INTEG_TEST) $@ ./agent/app)

out/test-artifacts/windows-stats-tests.exe: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call win-cgo-dockerbuild,$(GO_INTEG_TEST) $@ ./agent/stats)

GO_FUNCTIONAL_TEST = go test -tags functional -c -o
out/test-artifacts/linux-simple-tests: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call dockerbuild,$(GO_FUNCTIONAL_TEST) $@ ./agent/functional_tests/tests/generated/simpletests_unix)

out/test-artifacts/linux-handwritten-tests: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call dockerbuild,$(GO_FUNCTIONAL_TEST) $@ ./agent/functional_tests/tests)

out/test-artifacts/windows-simple-tests.exe: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call win-cgo-dockerbuild,$(GO_FUNCTIONAL_TEST) $@ ./agent/functional_tests/tests/generated/simpletests_windows)

out/test-artifacts/windows-handwritten-tests.exe: $(ALL_GO_FILES) .out-stamp .builder-image-stamp
	$(call win-cgo-dockerbuild,$(GO_FUNCTIONAL_TEST) $@ ./agent/functional_tests/tests)

##.PHONY: test-artifacts-windows test-artifacts-linux test-artifacts

WINDOWS_ARTIFACTS_TARGETS := out/test-artifacts/windows-engine-tests.exe out/test-artifacts/windows-stats-tests.exe
WINDOWS_ARTIFACTS_TARGETS += out/test-artifacts/windows-app-tests.exe out/test-artifacts/windows-simple-tests.exe
WINDOWS_ARTIFACTS_TARGETS += out/test-artifacts/windows-handwritten-tests.exe

LINUX_ARTIFACTS_TARGETS := out/test-artifacts/linux-engine-tests out/test-artifacts/linux-stats-tests
LINUX_ARTIFACTS_TARGETS += out/test-artifacts/linux-app-tests out/test-artifacts/linux-simple-tests
LINUX_ARTIFACTS_TARGETS += out/test-artifacts/linux-handwritten-tests

test-artifacts-windows: $(WINDOWS_ARTIFACTS_TARGETS)

test-artifacts-linux: $(LINUX_ARTIFACTS_TARGETS)

test-artifacts: test-artifacts-windows test-artifacts-linux

# Run our 'test' registry needed for integ and functional tests
test-registry: netkitten volumes-test squid awscli image-cleanup-test-images fluentd agent-introspection-validator taskmetadata-validator v3-task-endpoint-validator
	@./scripts/setup-test-registry

test-in-docker:
	docker build -f scripts/dockerfiles/Dockerfile.test -t "amazon/amazon-ecs-agent-test:make" .
	# Privileged needed for docker-in-docker so integ tests pass
	docker run --net=none -v "$(PWD):/go/src/github.com/aws/amazon-ecs-agent" --privileged "amazon/amazon-ecs-agent-test:make"

run-functional-tests: testnnp test-registry ecr-execution-role-image telemetry-test-image
	. ./scripts/shared_env && go test -tags functional -timeout=30m -v ./agent/functional_tests/...

.PHONY: build-image-for-ecr ecr-execution-role-image-for-upload upload-images replicate-images

build-image-for-ecr: netkitten volumes-test squid awscli image-cleanup-test-images fluentd taskmetadata-validator testnnp container-health-check-image telemetry-test-image ecr-execution-role-image-for-upload

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

get-cni-sources:
	git submodule update --init --checkout

cni-plugins: get-cni-sources .out-stamp
	@docker build -f scripts/dockerfiles/Dockerfile.buildCNIPlugins -t "amazon/amazon-ecs-build-cniplugins:make" .
	docker run --rm --net=none \
		-e GIT_SHORT_HASH=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git rev-parse --short=8 HEAD) \
		-e GIT_PORCELAIN=$(shell cd $(ECS_CNI_REPOSITORY_SRC_DIR) && git status --porcelain 2> /dev/null | wc -l | sed 's/^ *//') \
		-u "$(USERID)" \
		-v "$(PWD)/out/cni-plugins:/go/src/github.com/aws/amazon-ecs-cni-plugins/bin/plugins" \
		-v "$(ECS_CNI_REPOSITORY_SRC_DIR):/go/src/github.com/aws/amazon-ecs-cni-plugins" \
		"amazon/amazon-ecs-build-cniplugins:make"
	@echo "Built amazon-ecs-cni-plugins successfully."

run-integ-tests: test-registry gremlin container-health-check-image run-sudo-tests
	. ./scripts/shared_env && go test -race -tags integration -timeout=7m -v ./agent/engine/... ./agent/stats/... ./agent/app/...

run-sudo-tests:
	. ./scripts/shared_env && sudo -E ${GO_EXECUTABLE} test -race -tags sudo -timeout=1m -v ./agent/engine/...

.PHONY: codebuild
codebuild: test-artifacts .out-stamp
	$(MAKE) release TARGET_OS="linux"
	TARGET_OS="linux" ./scripts/local-save
	$(MAKE) docker-release TARGET_OS="windows"
	TARGET_OS="windows" ./scripts/local-save

netkitten:
	$(MAKE) -C misc/netkitten $(MFLAGS)

volumes-test:
	$(MAKE) -C misc/volumes-test $(MFLAGS)

# TODO, replace this with a build on dockerhub or a mechanism for the
# functional tests themselves to build this
.PHONY: squid awscli fluentd gremlin agent-introspection-validator taskmetadata-validator v3-task-endpoint-validator image-cleanup-test-images ecr-execution-role-image container-health-check-image telemetry-test-image
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

ecr-execution-role-image:
	$(MAKE) -C misc/ecr $(MFLAGS)

telemetry-test-image:
	$(MAKE) -C misc/telemetry $(MFLAGS)

container-health-check-image:
	$(MAKE) -C misc/container-health $(MFLAGS)

.PHONY: gocyclo
gocyclo:
	# Run gocyclo over all .go files in the agent, excluding vendor/, model/ and testutils/ directories, and all *_test.go and *_mocks.go files
	gocyclo -over 15 $(shell go list -f '{{$$p := .}}{{range $$f := .GoFiles}}{{$$p.Dir}}/{{$$f}} {{end}}' ./agent/... \
		| grep -v /vendor/ | grep -v /testutils/ | grep -v _test\.go$ | grep -v _mocks\.go$ | grep -v /model)

#TODO, create and add go vet target
.PHONY: static-check
static-check: gocyclo

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
	$(MAKE) -C $(ECS_CNI_REPOSITORY_SRC_DIR) clean
	-$(MAKE) -C misc/netkitten $(MFLAGS) clean
	-$(MAKE) -C misc/volumes-test $(MFLAGS) clean
	-$(MAKE) -C misc/gremlin $(MFLAGS) clean
	-$(MAKE) -C misc/testnnp $(MFLAGS) clean
	-$(MAKE) -C misc/image-cleanup-test-images $(MFLAGS) clean
	-$(MAKE) -C misc/agent-introspection-validator $(MFLAGS) clean
	-$(MAKE) -C misc/taskmetadata-validator $(MFLAGS) clean
	-$(MAKE) -C misc/v3-task-endpoint-validator $(MFLAGS) clean
	-$(MAKE) -C misc/container-health $(MFLAGS) clean
	-$(MAKE) -C misc/telemetry $(MFLAGS) clean
	-rm -f .get-deps-stamp
	-rm -f .builder-image-stamp
	-rm -f .out-stamp
	-rm -rf $(PWD)/bin

