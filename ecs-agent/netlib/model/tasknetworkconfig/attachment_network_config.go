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

// AttachmentNetworkConfig is the networking modelled from one task ENI
// attachment, before the task payload arrives.
type AttachmentNetworkConfig struct {
	// PrimaryNetNS is the task's primary network namespace. Nil when the
	// attached ENI is not the task's primary (index 0).
	PrimaryNetNS *NetworkNamespace

	// DaemonNetNS is the host's shared daemon namespace, which egresses over
	// the attached ENI. Nil on platforms without one.
	DaemonNetNS *NetworkNamespace
}
