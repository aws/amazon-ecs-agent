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

package container

// DockerEventType represents the type of docker events
type DockerEventType int

const (
	// ContainerStatusEvent represents the container status change events from docker
	// currently create, start, stop, die, restart and oom event will have this type
	ContainerStatusEvent DockerEventType = iota
	// ContainerHealthEvent represents the container health status event from docker
	// "health_status: unhealthy" and "health_status: healthy" will have this type
	ContainerHealthEvent
)

func (eventType DockerEventType) String() string {
	switch eventType {
	case ContainerStatusEvent:
		return "ContainerStatusChangeEvent"
	case ContainerHealthEvent:
		return "ContainerHealthChangeEvent"
	default:
		return "UNKNOWN"
	}
}
