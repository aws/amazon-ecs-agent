module github.com/aws/amazon-ecs-agent/dcgm-init

go 1.25.0

toolchain go1.25.9

require github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgmclient v0.0.0

replace github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgmclient => ../ecs-agent/gpu/dcgmclient

replace github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types => ../ecs-agent/gpu/types

replace github.com/aws/amazon-ecs-agent/ecs-agent => ../ecs-agent

require (
	github.com/NVIDIA/go-dcgm v0.0.0-20251203192032-7ac2f778d507 // indirect
	github.com/aws/amazon-ecs-agent/ecs-agent v0.0.0 // indirect
	github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types v0.0.0 // indirect
	github.com/bits-and-blooms/bitset v1.22.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/sys v0.46.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
