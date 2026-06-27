module github.com/aws/amazon-ecs-agent/dcgm-init

go 1.25.0

toolchain go1.25.9

require github.com/aws/amazon-ecs-agent/ecs-agent v0.0.0

replace github.com/aws/amazon-ecs-agent/ecs-agent => ../ecs-agent

replace github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types => ../ecs-agent/gpu/types

require (
	github.com/NVIDIA/go-dcgm v0.0.0-20260622182233-0740c4cbc959 // indirect
	github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types v0.0.0 // indirect
	github.com/bits-and-blooms/bitset v1.22.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
