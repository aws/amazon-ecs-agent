//go:build linux
// +build linux

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

package session

const (
	// AdditionalEBSVolumeTimeoutDurationInMs sets the duration that Linux will additionally
	// wait for the EBS volume to be staged. This value is set to zero for Linux since it
	// does not need any additional time to stage the volume and can work within the defined
	// timeout that the ControlPlane sets. This is a Windows specific value
	AdditionalEBSVolumeTimeoutDurationInMs = 0
)
