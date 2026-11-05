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

package meminfo

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// readMemInfo retrieves memory statistics of the host system and returns a
// Memory type.
func readMemInfo() (*Memory, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseMemInfo(file)
}

// parseMemInfo parses the /proc/meminfo file into
// a Memory object given an io.Reader to the file.
// Throws error if there are problems reading from the file
func parseMemInfo(reader io.Reader) (*Memory, error) {
	meminfo := &Memory{}
	scanner := bufio.NewScanner(reader)
	memAvailable := int64(-1)
	for scanner.Scan() {
		// Expected format: ["MemTotal:", "1234", "kB"]
		parts := strings.Fields(scanner.Text())

		// Sanity checks: Skip malformed entries.
		if len(parts) < 3 || parts[2] != "kB" {
			continue
		}

		// Convert to bytes.
		size, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		// Convert to KiB
		bytes := int64(size) * 1024

		switch parts[0] {
		case "MemTotal:":
			meminfo.MemTotal = bytes
		case "MemFree:":
			meminfo.MemFree = bytes
		case "MemAvailable:":
			memAvailable = bytes
		case "SwapTotal:":
			meminfo.SwapTotal = bytes
		case "SwapFree:":
			meminfo.SwapFree = bytes
		}
	}
	if memAvailable != -1 {
		meminfo.MemFree = memAvailable
	}

	// Handle errors that may have occurred during the reading of the file.
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return meminfo, nil
}
