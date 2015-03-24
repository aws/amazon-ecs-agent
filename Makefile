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
.PHONY: dev generate lint static test sources rpm

dev:
	./scripts/gobuild.sh dev

generate:
	PATH=$(PATH):$(shell pwd)/scripts go generate -v ./...

lint:
	./scripts/lint.sh

static:
	./scripts/gobuild.sh

test: generate lint
	go test -v -cover ./...

sources:
	tar -czf ./sources.tgz ecs-init scripts

srpm: sources
	rpmbuild -bs ecs-init.spec

rpm: sources
	rpmbuild -bb ecs-init.spec

get-deps:
	go get github.com/tools/godep
	go get golang.org/x/tools/cover

clean:
	-rm ./sources.tgz
	-rm ./amazon-ecs-init
	-rm ./ecs-init-*.src.rpm
	-rm ./ecs-init-* -r
	-rm ./BUILDROOT -r
	-rm ./x86_64 -r
