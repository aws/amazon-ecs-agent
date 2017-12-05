<!--
Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may
not use this file except in compliance with the License. A copy of the
License is located at

     http://aws.amazon.com/apache2.0/

or in the "license" file accompanying this file. This file is distributed
on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
express or implied. See the License for the specific language governing
permissions and limitations under the License.
-->


### Introduction 
All containers launched with `awsvpc` networking mode get a local IPv4 address from the `169.254.172.0/22`. This can be used to identify a container and the task that it belongs to based on its IPv4 address. A HTTP endpoint exposed by the ECS agent can in turn serve metadata to containers based on the request's IPv4 address. 

A benefit of providing a HTTP endpoint is that it makes it easier for containers to deal with failures using HTTP error codes. There are also no additional environment variable lookups, with the experience being very similar to [EC2 instance metadata service](docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html)  

### Overview of the solution
* Upon starting the `pause` container, ECS agent caches the IPv4 address allocated to it by the `ecs-ipam` plugin 
* A container in a task invokes the `GET 169.254.170.2/v2/metadata` API
* This request reaches the ECS agent via the `ecs` bridge on port `51679`
* ECS agent looks up the request's IPv4 address against the cache to determine the task that the container belongs to 
* ECS agent serves the metadata for the container

### API schema
The following APIs will be initially supported:

|Request Path|Response|Response Schema|
|----|----|----|
|`GET 169.254.170.2/v2/metadata`| Task metadata | https://github.com/aws/amazon-ecs-agent/blob/f79e4ca8f2d01b7e13e393fe836f10b06ede8d16/agent/handlers/types/v2/response.go#L28|
|`GET 169.254.170.2/v2/metadata/<container-id>`| Container metadata | https://github.com/aws/amazon-ecs-agent/blob/f79e4ca8f2d01b7e13e393fe836f10b06ede8d16/agent/handlers/types/v2/response.go#L44|
|`GET 169.254.170.2/v2/stats`| Task stats (Contains stats for all containers in the task) | Map of containers to container stats as per https://docs.docker.com/engine/api/v1.30/#operation/ContainerStats|
|`GET 169.254.170.2/v2/stats/<container-id>`| Container stats | Container stats as per https://docs.docker.com/engine/api/v1.30/#operation/ContainerStats|

### Sample application
A sample application for querying this endpoint can be found [here](https://github.com/aws/amazon-ecs-agent/blob/0d25cecc21513c8b411361295d79012f239be744/misc/taskmetadata-validator/taskmetadata-validator.go)
