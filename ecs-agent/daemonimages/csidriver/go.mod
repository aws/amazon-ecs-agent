module github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver

go 1.23.0

toolchain go1.24.2

require (
	github.com/container-storage-interface/spec v1.9.0
	github.com/golang/mock v1.6.0
	github.com/kubernetes-csi/csi-proxy/client v1.1.3
	github.com/stretchr/testify v1.9.0
	golang.org/x/sys v0.31.0
	google.golang.org/grpc v1.64.1
	k8s.io/apimachinery v0.30.1
	k8s.io/klog/v2 v2.120.1
	k8s.io/mount-utils v0.30.1
	k8s.io/utils v0.0.0-20240502163921-fe8a2dddb1d0
)

require (
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240401170217-c3f982113cda // indirect
	google.golang.org/protobuf v1.34.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
