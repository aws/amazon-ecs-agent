# Amazon ECS Agent - Functional Tests

This directory contains scripts to perform a functional test of a build of the
agent against the production Amazon EC2 Container Service.

These tests are meant to cover scenarios that are difficult to cover with `go test`.
The bulk of the tests in this repository may be found alongside the Go source
code.

These tests are meant to be run on an EC2 instance with a suitably powerful
role to interact with ECS.
You may be charged for the AWS resources utilized while running these tests.
It is not recommended to run these without understanding the implications of
doing so and it is certainly not recommended to run them on an AWS account
handling production work-loads.

## Setup

Before running these tests, you should build the ECS Agent and tag its image as
`amazon/amazon-ecs-agent:make`.

You should also run the registry the tests pull from. This can most easily be done via `make test-registry`.

## Running

These tests should be run on an EC2 instance able to correctly run the ECS
agent and access the ECS APIs.

The best way to run them is via the `make run-functional-tests` target.

Thay may also be manually run with `go test -tags functional -v ./...`,

### Envrionment Variable
In order to run Telemetry functional test in non Amazon Linux AMI environment, the following environment variables should be set:
    * CGROUP_PATH: cgroup path on the host, default value "/cgroup"
    * EXECDRIVER_PATH: execdriver path on the host, default value "/var/run/docker/execdriver"
