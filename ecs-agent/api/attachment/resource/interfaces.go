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

package resource

// EBSDiscovery is an interface used to find EBS volumes that are attached onto the host instance. It is implemented by
// EBSDiscoveryClient
type EBSDiscovery interface {
	ConfirmEBSVolumeIsAttached(deviceName, volumeID string) (string, error)
}

// GenericEBSAttachmentObject is an interface used to implement the Resource attachment objects that's saved within the agent state
type GenericEBSAttachmentObject interface {
	GetAttachmentProperties(key string) string
	EBSToString() string
	SetError(err error)
}
