CSI_DRIVER_VERSION?=v1.0.0
GO111MODULE=on
GOPATH=$(shell go env GOPATH)
BUILD_DATE=$(shell date -u -Iseconds)

OS?=linux
ARCH?=amd64

.PHONY: bin/csi-driver
bin/csi-driver:
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags "\
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/version.version=$(CSI_DRIVER_VERSION)\" \
		-X \"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/version.buildDate=$(BUILD_DATE)\"" \
		-o ./bin/csi-driver ./

.PHONY: test
test:
	go test -v -race -tags unit -timeout=60s ./...

# please include this `mockgen` into PATH
bin/mockgen:
	go install github.com/golang/mock/mockgen@v1.6.0

.PHONY: mockgen
mockgen: bin/mockgen
	./update-gomock

.PHONY: clean
clean:
	rm -rf bin/
