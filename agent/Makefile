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
