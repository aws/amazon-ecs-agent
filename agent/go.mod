module github.com/aws/amazon-ecs-agent/agent

go 1.12

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/aws/aws-sdk-go v1.36.0
	github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit v0.0.0-20190829210224-55d4fd2e6f35
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/containerd/cgroups v0.0.0-20170627184340-c3fc2b77b568
	github.com/containerd/containerd v1.3.2 // indirect
	github.com/containerd/continuity v0.0.0-20181023183536-c220ac4f01b8 // indirect
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.9.1
	github.com/deniswernert/udev v0.0.0-20140626150257-82fe5be8ca5f
	github.com/didip/tollbooth v3.0.2+incompatible
	github.com/docker/distribution v0.0.0-20181002220433-1cb4180b1a5b // indirect
	github.com/docker/docker v0.0.0-20200531234253-77e06fda0c94
	github.com/docker/go-connections v0.3.0
	github.com/docker/go-units v0.3.2
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/gogo/protobuf v1.1.1 // indirect
	github.com/golang/mock v1.1.1
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2
	github.com/gorilla/websocket v1.2.0
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/konsorten/go-windows-terminal-sequences v1.0.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runtime-spec v0.0.0-20170627113742-d349388c43b0
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pborman/uuid v0.0.0-20150603214016-ca53cad383ca
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v0.9.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.0.0-20181020173914-7e9e6cabbd39 // indirect
	github.com/prometheus/procfs v0.0.0-20181005140218-185b4288413d // indirect
	github.com/sirupsen/logrus v1.1.1 // indirect
	github.com/stretchr/testify v1.5.1
	github.com/vishvananda/netlink v0.0.0-20181108222139-023a6dafdcdf
	go.etcd.io/bbolt v1.3.6
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b
	golang.org/x/sys v0.0.0-20200923182605-d9f96fdee20d
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/time v0.0.0-20170927054726-6dc17368e09b // indirect
	golang.org/x/tools v0.0.0-20190524140312-2c0ae7006135
	google.golang.org/grpc v1.38.0 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)

replace (
	// Note: the following packages are downgraded explicitly to match the version we were using when we used dep, so that
	// dependency change is not coupled with migration to go mod. No other reason to keep them downgraded (if in the
	// future we need to downgrade dependency due to other reason, such as incompatibility with newer version, those
	// reasons should be noted down separately).
	github.com/Microsoft/go-winio => github.com/Microsoft/go-winio v0.4.7
	github.com/containernetworking/plugins => github.com/containernetworking/plugins v0.8.6
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20170731111925-d21964639418
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.0
	github.com/godbus/dbus => github.com/godbus/dbus v4.1.0+incompatible
	github.com/golang/mock => github.com/golang/mock v1.3.1-0.20190508161146-9fa652df1129
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.1
	github.com/jmespath/go-jmespath => github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af
	github.com/konsorten/go-windows-terminal-sequences => github.com/konsorten/go-windows-terminal-sequences v1.0.1
	github.com/pkg/errors v0.8.1 => github.com/pkg/errors v0.9.1
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910
	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.1.1
	github.com/stretchr/testify => github.com/stretchr/testify v1.2.2
	github.com/vishvananda/netlink => github.com/vishvananda/netlink v0.0.0-20170220200719-fe3b5664d23a
	github.com/vishvananda/netns => github.com/vishvananda/netns v0.0.0-20171111001504-be1fbeda1936
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20171113213409-9f005a07e0d3
	golang.org/x/net => golang.org/x/net v0.0.0-20171107184841-a337091b0525
	golang.org/x/sys => golang.org/x/sys v0.0.0-20171114162044-bf42f188b9bc
	golang.org/x/tools => golang.org/x/tools v0.0.0-20171114152239-bd4635fd2559
)
