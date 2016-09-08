# Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
.PHONY: dev generate lint static test build-mock-images sources rpm

dev:
	./scripts/gobuild.sh dev

generate:
	PATH=$(PATH):$(shell pwd)/scripts go generate -v ./...

lint:
	./scripts/lint.sh

static:
	./scripts/gobuild.sh

gotest:
	GOPATH="$(shell pwd)/ecs-init/Godeps/_workspace:$(GOPATH)" go test -short -v -cover ./...

test: generate lint gotest

test-in-docker:
	docker build -f scripts/dockerfiles/test.dockerfile -t "amazon/amazon-ecs-init-test:make" .
	docker run -v "$(shell pwd):/go/src/github.com/aws/amazon-ecs-init" "amazon/amazon-ecs-init-test:make"

build-mock-images:
	docker build -t "test.localhost/amazon/mock-ecs-agent" -f "scripts/dockerfiles/mock-agent.dockerfile" .
	docker build -t "test.localhost/amazon/wants-update" -f "scripts/dockerfiles/wants-update.dockerfile" .
	docker build -t "test.localhost/amazon/exit-success" -f "scripts/dockerfiles/exit-success.dockerfile" .

sources:
	./scripts/update-version.sh
	cp packaging/amazon-linux-ami/ecs-init.spec ecs-init.spec
	cp packaging/amazon-linux-ami/ecs.conf ecs.conf
	tar -czf ./sources.tgz ecs-init scripts

srpm: sources
	rpmbuild -bs ecs-init.spec

rpm: sources
	rpmbuild -bb ecs-init.spec

get-deps:
	go get github.com/tools/godep
	go get golang.org/x/tools/cover
	go get golang.org/x/tools/cmd/cover
	go get golang.org/x/tools/cmd/goimports

clean:
	-rm ecs-init.spec
	-rm ecs.conf
	-rm ./sources.tgz
	-rm ./amazon-ecs-init
	-rm ./ecs-init-*.src.rpm
	-rm ./ecs-init-* -r
	-rm ./BUILDROOT -r
	-rm ./x86_64 -r
