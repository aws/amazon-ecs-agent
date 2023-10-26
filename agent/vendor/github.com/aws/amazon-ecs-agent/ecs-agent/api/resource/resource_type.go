// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package resource

const (
	// EphemeralStorage is one of the resource types in the properties list of the attachment payload message for the
	// ephemeral storage.
	EphemeralStorage = "EphemeralStorage"
	// ElasticBlockStorage is one of the resource types in the properties list of the attachment payload message for the
	// EBS volume on firecracker.
	ElasticBlockStorage = "ElasticBlockStorage"
	// EBSTaskAttach is one of the attachment types in the attachment payload message for EBS attach tasks.
	EBSTaskAttach = "amazonebs"
)
