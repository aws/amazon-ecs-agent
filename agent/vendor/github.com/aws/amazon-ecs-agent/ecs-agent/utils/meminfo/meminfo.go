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
//
// This file is derived from github.com/docker/docker/pkg/meminfo (v25.0.6),
// Copyright 2013-2018 Docker, Inc., licensed under the Apache License 2.0.
// moby v29 does not publish pkg/meminfo as an importable module, so the agent
// carries its own copy.

// Package meminfo provides utilites to retrieve memory statistics of
// the host system.
package meminfo

// Read retrieves memory statistics of the host system and returns a
// Memory type. It is only supported on Linux and Windows, and returns an
// error on other platforms.
func Read() (*Memory, error) {
	return readMemInfo()
}

// Memory contains memory statistics of the host system.
type Memory struct {
	// Total usable RAM (i.e. physical RAM minus a few reserved bits and the
	// kernel binary code).
	MemTotal int64

	// Amount of free memory.
	MemFree int64

	// Total amount of swap space available.
	SwapTotal int64

	// Amount of swap space that is currently unused.
	SwapFree int64
}
