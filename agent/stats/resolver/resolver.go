// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package resolver

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

//go:generate mockgen -destination=mock/$GOFILE -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/stats/resolver ContainerMetadataResolver

// ContainerMetadataResolver defines methods to resolve meta-data.
type ContainerMetadataResolver interface {
	ResolveTask(string) (*apitask.Task, error)
	ResolveContainer(string) (*apicontainer.DockerContainer, error)
	ResolveTaskByARN(string) (*apitask.Task, error)
}
