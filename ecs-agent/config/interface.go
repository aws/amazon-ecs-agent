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

package config

// AgentConfigAccessor provides access to an agent config via a set of designated methods (listed below).
type AgentConfigAccessor interface {
	// AcceptInsecureCert returns whether clients validate SSL certificates.
	AcceptInsecureCert() bool
	// APIEndpoint returns the endpoint to make calls against.
	// (e.g., "ecs.us-east-1.amazonaws.com")
	APIEndpoint() string
	// AWSRegion returns the region to run in.
	// (e.g., "us-east-1")
	AWSRegion() string
	// Cluster returns the name or full ARN of the cluster that the Agent
	// should register the container instance into.
	Cluster() string
	// DefaultClusterName returns the name of the cluster that the container
	// instance should be registered into by default (i.e., if no cluster is
	// specified).
	// (e.g., "default")
	DefaultClusterName() string
	// External returns whether the Agent is running on external compute
	// capacity (i.e., outside of AWS).
	External() bool
	// InstanceAttributes returns a map of key/value pairs representing
	// attributes to be associated with the instance within the ECS service.
	// They are used to influence behavior such as launch placement.
	InstanceAttributes() map[string]string
	// NoInstanceIdentityDocument returns whether the Agent should register
	// the instance with instance identity document. When this method returns true,
	// it means that the Agent should not register the instance with instance identity document.
	// This is used to accommodate scenarios where the instance identity document is not
	// available or needed (e.g., when Agent is running on external compute capacity).
	NoInstanceIdentityDocument() bool
	// OSFamily returns the operating system family of the instance.
	// (e.g., "WINDOWS_SERVER_2019_CORE")
	OSFamily() string
	// OSType returns the operating system type of the instance.
	// (e.g., "windows")
	OSType() string
	// ReservedMemory returns the reduction (in MiB) of the memory
	// capacity of the instance that is reported to Amazon ECS.
	// It is used by ECS service to influence task placement and does NOT reserve
	// memory usage on the instance.
	ReservedMemory() uint16
	// ReservedPorts returns an array of ports which should be registered as
	// unavailable.
	ReservedPorts() []uint16
	// ReservedPortsUDP returns an array of UDP ports which should be registered
	// as unavailable.
	ReservedPortsUDP() []uint16
	// UpdateCluster updates the cluster that the Agent should register the
	// container instance into.
	UpdateCluster(cluster string)
}
