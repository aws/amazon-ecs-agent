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

package config

// OSType is the type of operating system where agent is running
const (
	OSType = "windows"
	// containerAdminUser is the admin username for any container on Windows.
	ContainerAdminUser = "ContainerAdministrator"
	// This is the path that will be used to store the local named pipe for CSI Proxy
	ManagedDaemonSocketPathHostRoot = "C:\\ProgramData\\Amazon\\ECS\\ebs-csi-driver"
	// This is the path that will be used to store the log file for the CSI Driver Managed Daemon
	ManagedDaemonLogPathHostRoot = "C:\\ProgramData\\Amazon\\ECS\\log\\daemons"
)
