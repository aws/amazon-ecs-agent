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

package utils

import (
	"bufio"
	"os"
	"strings"
)

type CPUInfo struct {
	Processors []Processor `json:"processors"`
}

type Processor struct {
	Flags []string `json:"flags"`
}

var (
	OpenFile = os.Open
)

func ReadCPUInfo(path string) (*CPUInfo, error) {
	file, err := OpenFile(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	var cpuinfo = CPUInfo{}
	var processor = &Processor{}

	for i, line := range lines {
		var key string
		var value string

		if len(line) == 0 && i != len(lines)-1 {
			// end of processor
			cpuinfo.Processors = append(cpuinfo.Processors, *processor)
			processor = &Processor{}
			continue
		} else if i == len(lines)-1 {
			cpuinfo.Processors = append(cpuinfo.Processors, *processor)
			continue
		}

		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}
		key = strings.TrimSpace(fields[0])
		value = strings.TrimSpace(fields[1])

		switch key {
		case "flags", "Features":
			processor.Flags = strings.FieldsFunc(value, func(r rune) bool {
				return r == ',' || r == ' '
			})
		}
	}
	return &cpuinfo, nil
}

// GetCPUFlags merges all processors' flags and return as a map. Returning map makes it
// easy to check whether a flag exists or not.
func GetCPUFlags(cpuInfo *CPUInfo) map[string]bool {
	flagMap := map[string]bool{}
	for _, proc := range cpuInfo.Processors {
		for _, flag := range proc.Flags {
			flagMap[flag] = true
		}
	}
	return flagMap
}
