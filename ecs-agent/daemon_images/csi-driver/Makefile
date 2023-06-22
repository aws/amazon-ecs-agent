CSI_DRIVER_VERSION?=v1.0.0
GO111MODULE=on
GOPATH=$(shell go env GOPATH)
BUILD_DATE=$(shell date -u -Iseconds)

SOURCEDIR=./
CSI_DRIVER_SOURCES=$(shell find $(SOURCEDIR) -name '*.go')

OS?=linux
ARCH?=amd64
.PHONY: build-image
build-image: bin/csi-driver
	OS=$(OS) ARCH=$(ARCH) ./build-image-as-tar

# Build binary for linux only
.PHONY: bin/csi-driver
bin/csi-driver: $(CSI_DRIVER_SOURCES)
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags "\
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/version.version=$(CSI_DRIVER_VERSION)\" \
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/version.buildDate=$(BUILD_DATE)\"" \
		-o ./bin/csi-driver ./

.PHONY: test
test:
	go test -v -race -tags unit -timeout=120s ./...

# please include this `mockgen` into PATH
bin/mockgen: | bin
	go install github.com/golang/mock/mockgen@v1.6.0

.PHONY: mockgen
mockgen: bin/mockgen
	./update-gomock

.PHONY: clean
clean:
	rm -rf bin/*
