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

package testconst

// This file contains constants that are commonly used when testing ACS session and responders. These constants
// should only be called in test files.
const (
	ClusterName          = "default"
	ContainerInstanceARN = "arn:aws:ecs:us-west-2:123456789012:container-instance/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE"
	TaskARN              = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
	MessageID            = "123"
	RandomMAC            = "00:0a:95:9d:68:16"
	WaitTimeoutMillis    = 1000
	InterfaceProtocol    = "default"
	GatewayIPv4          = "192.168.1.1/24"
	IPv4Address          = "ipv4"
)
