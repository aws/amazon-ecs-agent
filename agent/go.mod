module github.com/aws/amazon-ecs-agent/agent

go 1.19

require (
	github.com/aws/amazon-ecs-agent/ecs-agent v0.0.0-00010101000000-000000000000
	github.com/aws/aws-sdk-go v1.36.0
	github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit v0.0.0-20210308162251-8959c62cb8f9
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/containerd/cgroups v1.0.4
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.9.1
	github.com/deniswernert/udev v0.0.0-20170418162847-a12666f7b5a1
	github.com/docker/docker v20.10.23+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.6.0
	github.com/golang/mock v1.4.1
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.26.0
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	go.etcd.io/bbolt v1.3.5
	golang.org/x/net v0.8.0
	golang.org/x/sys v0.6.0
	golang.org/x/tools v0.6.0
	google.golang.org/grpc v1.52.0
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Microsoft/hcsshim v0.8.25 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/cilium/ebpf v0.6.2 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/didip/tollbooth v4.0.2+incompatible // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/moby/sys/mount v0.3.3 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/term v0.0.0-20221205130635-1aeaba878587 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.25.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae // indirect
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	google.golang.org/genproto v0.0.0-20221118155620-16455021b5e6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/aws/amazon-ecs-agent/ecs-agent => ../ecs-agent
