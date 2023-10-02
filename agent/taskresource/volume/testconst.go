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

package volume

// This file contains constants that are commonly used when testing with EBS volumes for tasks. These constants
// should only be called in test files.
const (
	TestAttachmentType       = "EBSTaskAttach"
	TestVolumeId             = "vol-12345"
	TestVolumeSizeGib        = "10"
	TestSourceVolumeHostPath = "taskarn_vol-12345"
	TestVolumeName           = "test-volume"
	TestFileSystem           = "ext4"
	TestDeviceName           = "/dev/nvme1n1"
)
