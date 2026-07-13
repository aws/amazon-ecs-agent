module github.com/aws/amazon-ecs-agent/dcgm-init

go 1.26.0

require (
	github.com/aws/amazon-ecs-agent/ecs-agent v0.0.0
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/golang/mock v1.6.0
	github.com/stretchr/testify v1.12.1
)

replace github.com/aws/amazon-ecs-agent/ecs-agent => ../ecs-agent

require (
	github.com/NVIDIA/go-dcgm v0.0.0-20260622182233-0740c4cbc959 // indirect
	github.com/bits-and-blooms/bitset v1.22.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.49.0 // indirect
)
