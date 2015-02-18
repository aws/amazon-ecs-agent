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

package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

type testTaskStatus struct {
	SomeStatus TaskStatus `json:"status"`
}

func TestUnmarshalTaskStatus(t *testing.T) {
	status := TaskStatusNone

	err := json.Unmarshal([]byte(`"RUNNING"`), &status)
	if err != nil {
		t.Error(err)
	}
	if status != TaskRunning {
		t.Error("RUNNING should unmarshal to RUNNING, not " + status.String())
	}

	var test testTaskStatus
	err = json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != TaskStopped {
		t.Error("STOPPED should unmarshal to STOPPED, not " + test.SomeStatus.String())
	}
}

type testContainerStatus struct {
	SomeStatus ContainerStatus `json:"status"`
}

func TestUnmarshalContainerStatus(t *testing.T) {
	status := ContainerStatusNone

	err := json.Unmarshal([]byte(`"RUNNING"`), &status)
	if err != nil {
		t.Error(err)
	}
	if status != ContainerRunning {
		t.Error("RUNNING should unmarshal to RUNNING, not " + status.String())
	}

	var test testContainerStatus
	err = json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != ContainerStopped {
		t.Error("STOPPED should unmarshal to STOPPED, not " + test.SomeStatus.String())
	}
}

type testContainerOverrides struct {
	SomeContainerOverrides ContainerOverrides `json:"overrides"`
}

type testContainerOverrideInput struct {
	SomeContainerOverrides string `json:"overrides"`
}

func TestUnmarshalContainerOverrides(t *testing.T) {
	overrides := &ContainerOverrides{}

	err := json.Unmarshal([]byte(`{"command": ["sh", "-c", "sleep 300"]}`), &overrides)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(overrides.Command, &[]string{"sh", "-c", "sleep 300"}) {
		t.Error("Unmarshalled wrong result", overrides.Command)
	}

	var overrides3 testContainerOverrides
	err = json.Unmarshal([]byte(`{"overrides":{"command":["sh", "-c", "sleep 15"]}}`), &overrides3)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(overrides3.SomeContainerOverrides.Command, &[]string{"sh", "-c", "sleep 15"}) {
		t.Error("unmarshalled wrong result", overrides3)
	}

	overrides2 := ContainerOverrides{Command: &[]string{"sh", "-c", "sleep 1"}}

	strOverrides, err := json.Marshal(overrides2)
	if err != nil {
		t.Error(err)
	}
	input := testContainerOverrideInput{SomeContainerOverrides: string(strOverrides)}
	strInput, err := json.Marshal(input)
	if err != nil {
		t.Error(err)
	}

	var overrides4 testContainerOverrides
	err = json.Unmarshal(strInput, &overrides4)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(overrides4.SomeContainerOverrides.Command, &[]string{"sh", "-c", "sleep 1"}) {
		t.Error("Unmarshalled wrong result", overrides4)
	}

	// Test that marshalling an unknown key fails hard
	var overrides5 testContainerOverrides
	err = json.Unmarshal([]byte(`{"overrides":{"command":["ash","-c","sleep 1"],"containerPlanet":"mars"}}`), &overrides5)
	if err == nil {
		t.Error("No error on unknown json field containerPlanet")
	}

	// Test the same for the string
	err = json.Unmarshal([]byte(`{"overrides":"{\"command\":[\"ash\",\"-c\",\"sleep 1\"],\"containerPlanet\":\"mars\"}"}`), &overrides5)
	if err == nil {
		t.Error("No error for unknown json field in string, containerPlanet")
	}

	// Now error cases
	err = json.Unmarshal([]byte(`{"overrides":"a string that can't be json unmarshalled }{"}`), &overrides5)
	if err == nil {
		t.Error("No error when unmarshalling an invalid json string")
	}

	err = json.Unmarshal([]byte(`{"overrides": ["must be a string or object"] }`), &overrides5)
	if err == nil {
		t.Error("No error when unmarshalling a really invalid json string")
	}
}

func TestMarshalUnmarshalTaskVolumes(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			TaskVolume{Name: "1", Volume: &EmptyHostVolume{}},
			TaskVolume{Name: "2", Volume: &FSHostVolume{FSSourcePath: "/path"}},
		},
	}

	marshal, err := json.Marshal(task)
	if err != nil {
		t.Fatal("Could not marshal: ", err)
	}

	var out Task
	err = json.Unmarshal(marshal, &out)
	if err != nil {
		t.Fatal("Could not unmarshal: ", err)
	}

	if len(out.Volumes) != 2 {
		t.Fatal("Incorrect number of volumes")
	}

	var v1, v2 TaskVolume

	for _, v := range out.Volumes {
		if v.Name == "1" {
			v1 = v
		} else {
			v2 = v
		}
	}

	if _, ok := v1.Volume.(*EmptyHostVolume); !ok {
		t.Error("Expected v1 to be an empty volume")
	}

	fs, ok := v2.Volume.(*FSHostVolume)
	if !ok || fs.FSSourcePath != "/path" {
		t.Error("Unmarshaled v2 didn't match marshalled v2")
	}
}
