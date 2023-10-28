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

package attachment

const (
	// AttachmentNone is zero state of a task when received attach message from acs
	AttachmentNone AttachmentStatus = iota
	// AttachmentAttached represents that an attachment has shown on the host
	AttachmentAttached
	// AttachmentDetached represents that an attachment has been actually detached from the host
	AttachmentDetached
)

// AttachmentStatus is an enumeration type for attachment state
type AttachmentStatus int32

var attachmentStatusMap = map[string]AttachmentStatus{
	"NONE":     AttachmentNone,
	"ATTACHED": AttachmentAttached,
	"DETACHED": AttachmentDetached,
}

// String return the string value of the attachment status
func (attachStatus *AttachmentStatus) String() string {
	for k, v := range attachmentStatusMap {
		if v == *attachStatus {
			return k
		}
	}
	return "NONE"
}

// ShouldSend returns whether the status should be sent to backend
func (attachStatus *AttachmentStatus) ShouldSend() bool {
	return *attachStatus == AttachmentAttached
}
