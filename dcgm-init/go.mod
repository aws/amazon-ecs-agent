module github.com/aws/amazon-ecs-agent/dcgm-init

go 1.25.0

toolchain go1.25.9

require (
	github.com/aws/amazon-ecs-agent/ecs-agent v0.0.0
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
)

replace github.com/aws/amazon-ecs-agent/ecs-agent => ../ecs-agent

require golang.org/x/sys v0.46.0 // indirect
