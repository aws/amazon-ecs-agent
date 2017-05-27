// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"text/template"

	"golang.org/x/tools/imports"
)

var simpleTestPattern = `
// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package simpletest is an auto-generated set of tests defined by the json
// descriptions in testdata/simpletests.
//
// This file should not be edited; rather you should edit the generator instead
package simpletest

import (
	"testing"
	"time"
	"os"

	. "github.com/aws/amazon-ecs-agent/test/util"
)

{{ range $i,$el := $ }}

// Test{{ $el.Name }} {{ $el.Description }}
func Test{{ $el.Name }}(t *testing.T) {
	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" { t.Parallel() }
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "{{ $el.TaskDefinition }}")
	if err != nil {
		t.Fatal("Could not start task", err)
	}
	timeout, err := time.ParseDuration("{{ $el.Timeout }}")
	if err != nil {
		t.Fatal("Could not parse timeout", err)
	}
	err = testTask.WaitStopped(timeout)
	if err != nil {
		t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
	}

	{{ range $name, $code := $el.ExitCodes }}
	if exit, ok := testTask.ContainerExitcode("{{$name}}"); !ok || exit != {{ $code }} {
		t.Errorf("Expected {{$name}} to exit with {{$code}}; actually exited (%v) with %v", ok, exit)
	}
	{{ end }}
}
{{ end }}
`

func main() {
	if len(os.Args) != 2 {
		panic("Must have exactly one argument; the output file")
	}

	type simpleTestMetadata struct {
		Name           string
		Description    string
		TaskDefinition string
		Timeout        string
		ExitCodes      map[string]int
		Tags           []string
	}

	_, filename, _, _ := runtime.Caller(0)
	metadataFiles, err := filepath.Glob(filepath.Join(path.Dir(filename), "..", "testdata", "simpletests", "*.json"))
	if err != nil || len(metadataFiles) == 0 {
		panic("No tests found" + err.Error())
	}

	testMetadatas := make([]simpleTestMetadata, len(metadataFiles))
	for i, f := range metadataFiles {
		data, err := ioutil.ReadFile(f)
		if err != nil {
			panic("Cannot read file " + f)
		}
		err = json.Unmarshal(data, &testMetadatas[i])
		if err != nil {
			panic("Cannot parse " + f + ": " + err.Error())
		}
	}

	simpleTests := template.Must(template.New("simpleTest").Parse(simpleTestPattern))
	output := bytes.NewBuffer([]byte{})
	err = simpleTests.Execute(output, testMetadatas)
	if err != nil {
		panic(err)
	}
	formattedOutput, err := imports.Process("", output.Bytes(), nil)
	if err != nil {
		fmt.Println(string(output.Bytes()))
		panic(err)
	}

	// Add '.go' so the arg can be used with 'go run' as well, without being interpreted as a file to run
	outputFile, err := os.Create(os.Args[1] + ".go")
	if err != nil {
		panic(err)
	}
	outputFile.Write(formattedOutput)
}
