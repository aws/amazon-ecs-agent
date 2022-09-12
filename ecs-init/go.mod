module github.com/aws/amazon-ecs-agent/ecs-init

go 1.17

require (
	github.com/NVIDIA/gpu-monitoring-tools v0.0.0-20180829222009-86f2a9fac6c5
	github.com/aws/aws-sdk-go v1.36.0
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/docker/go-plugins-helpers v0.0.0-20181025120712-1e6269c305b8
	github.com/fsouza/go-dockerclient v0.0.0-20170830181106-98edf3edfae6
	github.com/golang/mock v1.3.1-0.20190508161146-9fa652df1129
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.2.2
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20160622173216-fa152c58bc15 // indirect
	github.com/Microsoft/go-winio v0.4.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/Sirupsen/logrus v0.11.6-0.20170515105910-5e5dc898656f // indirect
	github.com/coreos/go-systemd v0.0.0-00010101000000-000000000000 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/docker v1.4.2-0.20170530175538-4f55e390c4e5 // indirect
	github.com/docker/go-connections v0.2.2-0.20170331145122-e15c02316c12 // indirect
	github.com/docker/go-units v0.3.2 // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc0.0.20170510163354-eaa60544f31c // indirect
	github.com/opencontainers/image-spec v1.0.0-rc6.0.20170525204040-4038d4391fe9 // indirect
	github.com/opencontainers/runc v1.0.0-rc3.0.20170530161907-a6906d5a531a // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b // indirect
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f // indirect
	golang.org/x/text v0.3.7 // indirect
)

replace github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.0.0

replace golang.org/x/net => golang.org/x/net v0.0.0-20170529214944-3da985ce5951

replace golang.org/x/sys => golang.org/x/sys v0.0.0-20170529185110-b90f89a1e7a9

replace github.com/jmespath/go-jmespath => github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af

replace github.com/pkg/errors => github.com/pkg/errors v0.8.1-0.20170505043639-c605e284fe17
