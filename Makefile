# Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the
# "License"). You may not use this file except in compliance
#  with the License. A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.
VERSION := $(shell git describe --tags | sed -e 's/v//' -e 's/-.*//')
DEB_SIGN ?= 1

.PHONY: dev generate lint static test build-mock-images sources rpm srpm govet

dev:
	./scripts/gobuild.sh dev

generate:
	PATH=$(PATH):$(shell pwd)/scripts go generate -v ./...

lint:
	./scripts/lint.sh

static:
	./scripts/gobuild.sh

govet:
	go vet $(shell go list ./ecs-init/...)

gotest:
	go test -count=1 -short -v -cover ./...

test: generate lint govet gotest

# all .go files in the ecs-init
GOFILES:=$(shell go list -f '{{$$p := .}}{{range $$f := .GoFiles}}{{$$p.Dir}}/{{$$f}} {{end}}' ./ecs-init/...)

.PHONY: gocyclo
gocyclo:
	# Run gocyclo over all .go files
	gocyclo -over 12 ${GOFILES}

GOFMTFILES:=$(shell find ./ecs-init -not -path './ecs-init/vendor/*' -type f -iregex '.*\.go')

.PHONY: importcheck
importcheck:
	$(eval DIFFS:=$(shell goimports -l $(GOFMTFILES)))
	@if [ -n "$(DIFFS)" ]; then echo "Files incorrectly formatted. Fix formatting by running goimports:"; echo "$(DIFFS)"; exit 1; fi

.PHONY: static-check
static-check: gocyclo govet importcheck

test-in-docker:
	docker build -f scripts/dockerfiles/test.dockerfile -t "amazon/amazon-ecs-init-test:make" .
	docker run -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-init" "amazon/amazon-ecs-init-test:make"

build-mock-images:
	docker build -t "test.localhost/amazon/mock-ecs-agent" -f "scripts/dockerfiles/mock-agent.dockerfile" .
	docker build -t "test.localhost/amazon/wants-update" -f "scripts/dockerfiles/wants-update.dockerfile" .
	docker build -t "test.localhost/amazon/exit-success" -f "scripts/dockerfiles/exit-success.dockerfile" .

sources.tgz:
	./scripts/update-version.sh
	cp packaging/amazon-linux-ami/ecs-init.spec ecs-init.spec
	cp packaging/amazon-linux-ami/ecs.conf ecs.conf
	cp packaging/amazon-linux-ami/ecs.service ecs.service
	tar -czf ./sources.tgz ecs-init scripts

# Hook to perform preparation steps prior to the sources target.
prepare-sources::

sources: prepare-sources sources.tgz

.srpm-done: sources.tgz
	test -e SOURCES || ln -s . SOURCES
	rpmbuild --define "%_topdir $(PWD)" -bs ecs-init.spec
	find SRPMS/ -type f -exec cp {} . \;
	touch .srpm-done

srpm: .srpm-done

.rpm-done: sources.tgz
	test -e SOURCES || ln -s . SOURCES
	rpmbuild --define "%_topdir $(PWD)" -bb ecs-init.spec
	find RPMS/ -type f -exec cp {} . \;
	touch .rpm-done

rpm: .rpm-done

ubuntu-trusty:
	cp packaging/ubuntu-trusty/ecs.conf ecs.conf
	tar -czf ./amazon-ecs-init_${VERSION}.orig.tar.gz ecs-init ecs.conf scripts README.md
	mkdir -p BUILDROOT
	cp -r packaging/ubuntu-trusty/debian BUILDROOT/debian
	cp -r ecs-init BUILDROOT
	cp packaging/ubuntu-trusty/ecs.conf BUILDROOT
	cp -r scripts BUILDROOT
	cp README.md BUILDROOT
	cd BUILDROOT && debuild $(shell [ "$(DEB_SIGN)" -ne "0" ] || echo "-uc -us")

get-deps:
	go get golang.org/x/lint/golint
	go get golang.org/x/tools/cover
	go get golang.org/x/tools/cmd/cover
	go get github.com/fzipp/gocyclo
	go get golang.org/x/tools/cmd/goimports

clean:
	-rm -f ecs-init.spec
	-rm -f ecs.conf
	-rm -f ecs.service
	-rm -f ./sources.tgz
	-rm -f ./amazon-ecs-init
	-rm -f ./ecs-agent-*.tar
	-rm -f ./ecs-init-*.src.rpm
	-rm -rf ./ecs-init-*
	-rm -rf ./BUILDROOT BUILD RPMS SRPMS SOURCES SPECS
	-rm -rf ./x86_64
	-rm -f ./amazon-ecs-init_${VERSION}*
	-rm -f .srpm-done .rpm-done
