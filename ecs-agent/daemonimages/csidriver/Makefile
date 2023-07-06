CSI_DRIVER_VERSION="v$(shell cat ./container/VERSION)"
GO111MODULE=on
GOPATH=$(shell go env GOPATH)
BUILD_DATE=$(shell date -u -Iseconds)

OS?=linux
ARCH?=amd64

.PHONY: bin/csi-driver
bin/csi-driver:
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags "\
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/version.version=$(CSI_DRIVER_VERSION)\" \
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/version.buildDate=$(BUILD_DATE)\"" \
		-o ./bin/csi-driver ./

.PHONY: build-csi-driver
build-csi-driver: bin/csi-driver
	./build-csi-driver-image
	$(MAKE) clean

.PHONY: mockgen
mockgen:
	./update-gomock

.PHONY: clean
clean:
	rm -rf bin/
