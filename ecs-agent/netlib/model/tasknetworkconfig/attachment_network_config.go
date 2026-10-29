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

package tasknetworkconfig

// AttachmentNetworkConfig is the networking a platform can model from a task
// ENI attachment alone, ahead of the task payload.
type AttachmentNetworkConfig struct {
	// TaskNetNS is the task namespace the attached interface establishes. A
	// namespace takes its name and default route from its primary interface,
	// so only the task's primary interface establishes one; nil for any other.
	TaskNetNS *NetworkNamespace

	// DaemonNetNS is the host's shared daemon namespace, modelled on the
	// attached interface's address families and resolvers. Nil on platforms
	// that have no shared daemon namespace.
	DaemonNetNS *NetworkNamespace
}
