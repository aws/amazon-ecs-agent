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

package engine

import "time"

// unstage retries are ultimately limited by successful unstage or by the unstageVolumeTimeout. The value is
// set to 10 minutes for Windows. As part of our metrics analysis, the unstaging on Windows takes longer
// and giving the unstaging workflow to complete cleanly leaves the EC2 instance in a good state.
const unstageVolumeTimeout = 600 * time.Second
