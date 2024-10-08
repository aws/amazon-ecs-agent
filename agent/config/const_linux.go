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

package config

const (
	// OSType is the type of operating system where agent is running
	OSType = "linux"
	// This is the path that will be used for the csi-driver socker
	ManagedDaemonSocketPathHostRoot = "/var/run/ecs"
	// This is the path that will be used to store the log file for the CSI Driver Managed Daemon
	ManagedDaemonLogPathHostRoot = "/log/daemons"
)
