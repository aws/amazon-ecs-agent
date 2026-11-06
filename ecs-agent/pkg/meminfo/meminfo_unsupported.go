// This file is derived from the moby/moby (Docker) project.
// The original code may be found at:
// https://github.com/moby/moby/blob/master/pkg/meminfo/meminfo_unsupported.go
//
// Copyright 2013-2018 Docker, Inc. Licensed under the Apache License, Version 2.0.
//
// Modifications are Copyright Amazon.com Inc. or its affiliates. Licensed under the Apache License 2.0.
//
// Vendored into the ECS agent because moby v29 no longer publishes pkg/meminfo
// as a supported public Go module (it lives only in the non-importable
// github.com/moby/moby/v2 root module). See .scratch/docker-moby-migration ticket 08.

//go:build !linux && !windows

package meminfo

import "errors"

// readMemInfo is not supported on platforms other than linux and windows.
func readMemInfo() (*Memory, error) {
	return nil, errors.New("platform and architecture is not supported")
}
