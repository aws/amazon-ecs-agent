# Amazon ECS Agent - Integration Tests

This directory contains scripts to perform a high-level integration test of a
build of the agent against the production Amazon EC2 Container Service.

These tests are meant to cover scenarios that are difficult to cover with `go test`.
The bulk of the tests in this repository may be found alongside the Go source
code.

These tests are meant to be run on an EC2 instance with a suitably powerful
role to interact with ECS.
You will be charged for the AWS resources utilized while running these tests.
It is not recommended to run these without understanding the implications of
doing so and it is certainly not recommended to run them on an AWS account
handling production work-loads.

## Setup

Before running these tests, Docker should be running and an image of the ECS
Agent should be available tagged as `amazon/amazon-ecs-agent:latest`.

## Running

These tests should be run on an EC2 instance able to correctly run the ECS
agent and access the ECS APIs.

They may be run with `go test -v ./...`
