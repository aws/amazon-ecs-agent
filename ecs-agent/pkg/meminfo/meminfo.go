// This file is derived from the moby/moby (Docker) project.
// The original code may be found at:
// https://github.com/moby/moby/blob/master/pkg/meminfo/meminfo.go
//
// Copyright 2013-2018 Docker, Inc. Licensed under the Apache License, Version 2.0.
//
// Modifications are Copyright Amazon.com Inc. or its affiliates. Licensed under the Apache License 2.0.
//
// Vendored into the ECS agent because moby v29 no longer publishes pkg/meminfo
// as a supported public Go module (it lives only in the non-importable
// github.com/moby/moby/v2 root module). See .scratch/docker-moby-migration ticket 08.

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
