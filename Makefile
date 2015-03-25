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
.PHONY: gobuild generate lint static test sources rpm

cwd:=$(shell pwd)

gobuild:
	cd ecs-init && godep go build -o ../amz-ecs-init

generate:
	PATH=$(PATH):$(cwd)/scripts go generate -v ./...

lint:
	$(cwd)/scripts/lint.sh

static:
	cd ecs-init && CGO_ENABLED=0 godep go build -a -x -ldflags '-s' -o ../amz-ecs-init

test: generate lint
	go test -v -cover ./...

sources: static

rpm: sources
	rpmbuild -bb ecs-init.spec

get-deps:
	go get github.com/tools/godep
	go get golang.org/x/tools/cover