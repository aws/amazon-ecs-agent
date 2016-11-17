// Copyright 2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

const (
	projectVendor         = `github.com/aws/amazon-ecs-agent/agent/vendor`
	copyrightHeaderFormat = "// Copyright 2015-%v Amazon.com, Inc. or its affiliates. All Rights Reserved."
	licenseBlock          = `
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

  `
)

func main() {
	if len(os.Args) != 4 {
		usage()
		os.Exit(1)
	}
	packageName := os.Args[1]
	interfaces := os.Args[2]
	outputPath := os.Args[3]
	re := regexp.MustCompile("(?m)[\r\n]+^.*" + projectVendor + ".*$")

	copyrightHeader := fmt.Sprintf(copyrightHeaderFormat, time.Now().Year())

	path, _ := filepath.Split(outputPath)
	err := os.MkdirAll(path, os.ModeDir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	mockgen := exec.Command("mockgen", packageName, interfaces)
	mockgenOut, err := mockgen.Output()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sanitized := re.ReplaceAllString(string(mockgenOut), "")

	withHeader := copyrightHeader + licenseBlock + sanitized

	goimports := exec.Command("goimports")
	goimports.Stdin = bytes.NewBufferString(withHeader)
	goimports.Stdout = outputFile
	err = goimports.Run()
	outputFile.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(os.Args[0], " PACKAGE INTERFACE_NAMES OUTPUT_FILE")
}
