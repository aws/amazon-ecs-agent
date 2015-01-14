// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import "fmt"
import "github.com/aws/amazon-ecs-agent/agent/api"

type ContainerNotFound struct {
	TaskArn       string
	ContainerName string
}

func (cnferror ContainerNotFound) Error() string {
	return fmt.Sprintf("Could not find container '%s' in task '%s'", cnferror.ContainerName, cnferror.TaskArn)
}

type DockerContainerChangeEvent struct {
	DockerId string
	Image    string
	Status   api.ContainerStatus
}
