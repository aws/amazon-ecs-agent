//go:build windows
// +build windows

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
	// AdditionalEBSVolumeTimeoutDurationInMs sets the duration that Windows will additionally
	// wait for the EBS volume to be staged. We are allowing another 5 minutes for Windows to
	// complete the staging for the EBS volume.
	AdditionalEBSVolumeTimeoutDurationInMs = 300000
)
