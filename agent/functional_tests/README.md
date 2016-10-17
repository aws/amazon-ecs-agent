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
In order to run Telemetry functional test in non Amazon Linux AMI environment,
the following environment variables should be set:
  * CGROUP_PATH: cgroup path on the host, default value "/cgroup"
  * EXECDRIVER_PATH: execdriver path on the host, default value "/var/run/docker/execdriver"

In order to run TaskIamRole functional test, the following steps should be done first:
  * Run command: `sysctl -w net.ipv4.conf.all.route_localnet=1` and
    `iptables -t nat -A PREROUTING -p tcp -d 169.254.170.2 --dport 80 -j DNAT --to-destination 127.0.0.1:51679`.
  * Set the environment variable to enable the test under default network mode: `export TEST_TASK_IAM_ROLE=true`.
  * Set the envrionment variable of IAM roles the test will use: `export TASK_IAM_ROLE_ARN="iam role arn"`,
  the role should have the `ec2:DescribeRegions` permission and have the trust relationship with "ecs-tasks.amazonaws.com".
  Or if the `TASK_IAM_ROLE_ARN` isn't set, it will use the IAM role attached to the instance profile. In this case,
  except the permissions required before, the IAM role should also have `iam:GetInstanceProfile` permission.
  * Testing under net=host network mode requires additional command:
    `iptables -t nat -A OUTPUT -d 169.254.170.2 -p tcp -m tcp --dport 80 -j REDIRECT --to-ports 51679` and
    `export TEST_TASK_IAM_ROLE_NET_HOST=true`

## Windows Setup

Set the following environment variable.
  * $env:ECS_WINDOWS_TEST_DIR=##Path where the agent binary is present.
  * For performing the IAM roles test, perform the following additional tasks.
    ** `$env:TEST_TASK_IAM_ROLE="true"`
    ** `Set environment variable AWS_REGION. For example, `$env:AWS_REGION="us-east-1"`
    ** devcon.exe should be present in system environment variable PATH.    

## Running

Running the command run-functional-tests.ps1 from the scripts directory
