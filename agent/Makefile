BUILD_PLATFORM:=$(shell uname -m)

ifeq (${BUILD_PLATFORM},aarch64)
	GOARCH=arm64
else
	GOARCH=amd64
endif

.PHONY: gomodule-on
gomodule-on:
	GO111MODULE=on

# all .go files in the agent, excluding vendor/, model/ and testutils/ directories, and all *_test.go and *_mocks.go files
GOFILES:=$(shell go list -f '{{$$p := .}}{{range $$f := .GoFiles}}{{$$p.Dir}}/{{$$f}} {{end}}' ./... \
		| grep -v /testutils/ | grep -v _test\.go$ | grep -v _mocks\.go$ | grep -v /model)

.PHONY: gocyclo
gocyclo:
	# Run gocyclo over all .go files
	gocyclo -over 15 ${GOFILES}

.PHONY: govet
govet:
	go vet $(shell go list ./... | grep -v /testutils/ | grep -v _test\.go$ | grep -v /mocks | grep -v /model)

.PHONY: gogenerate-check
gogenerate-check:
	go generate -x ./...
	$(MAKE) goimports

GOFMTFILES:=$(shell find . -not -path './vendor/*' -not -path './tools.go' -type f -iregex '.*\.go')

.PHONY: goimports
goimports:
	goimports -w $(GOFMTFILES)

gogenerate:
	go generate -x ./...
	$(MAKE) goimports

.PHONY: gogenerate-check
gogenerate-check: gogenerate
	# check that gogenerate does not generate a diff.
	git diff --exit-code

.PHONY: goimports
goimports:
	goimports -w $(GOFMTFILES)

.PHONY: importcheck
importcheck:
	$(eval DIFFS:=$(shell goimports -l $(GOFMTFILES)))
	@if [ -n "$(DIFFS)" ]; then echo "Files incorrectly formatted. Fix formatting by running goimports:"; echo "$(DIFFS)"; exit 1; fi

.PHONY: static-check
static-check: gomodule-on gocyclo govet importcheck gogenerate-check
	# use default checks of staticcheck tool, except style checks (-ST*) and depracation checks (-SA1019)
	# depracation checks have been left out for now; removing their warnings requires error handling for newer suggested APIs, changes in function signatures and their usages.
	# https://github.com/dominikh/go-tools/tree/master/cmd/staticcheck
	staticcheck -tests=false -checks "inherit,-ST*,-SA1019" ./...

# 'go' may not be on the $PATH for sudo tests
GO_EXECUTABLE=$(shell command -v go 2> /dev/null)

# VERBOSE includes the options that make the test opt loud
VERBOSE=-v -cover

# -count=1 runs the test without using the build cache.  The build cache can
# provide false positives when running integ tests, so we err on the side of
# caution. See `go help test`
# unit tests include the coverage profile
GOTEST=${GO_EXECUTABLE} test -count=1 ${VERBOSE}

# -race sometimes causes compile issues on Arm
ifneq (${BUILD_PLATFORM},aarch64)
	GOTEST += -race
endif

.PHONY: test
test: gomodule-on
	${GOTEST} -tags unit -coverprofile cover.out -timeout=60s ./...
	go tool cover -func cover.out > coverprofile.out

.PHONY: test-silent
test-silent: gomodule-on
	$(eval VERBOSE=)
	${GOTEST} -tags unit -coverprofile cover.out -timeout=60s ./...
	go tool cover -func cover.out > coverprofile.out

.PHONY: benchmark-test
benchmark-test: gomodule-on
	go test -run=XX -bench=. ./...
