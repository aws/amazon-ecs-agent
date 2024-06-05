//go:build windows
// +build windows

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

package daemonmanager

import (
	"io/fs"
)

const (
	// This is the daemon UID that is used to track the ManagedDaemons
	daemonUID = 0
	// This is the permission that is set on the named pipes created for the ManagedDaemon
	daemonMountPermission fs.FileMode = 0755
	// This is the permission set that is used for setting the access control on the log file
	daemonLogPermission fs.FileMode = 0777
)
