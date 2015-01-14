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

package acs

import (
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

func TestParseResponseLine(t *testing.T) {
	client := testClient()

	line := []byte(`{}`)
	response, skip, err := client.parseResponseLine(line)
	if err == nil {
		t.Error("Empty response should not be valid")
	}
	if !skip {
		t.Error("Didn't skip the empty response")
	}

	line = []byte(`{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn1","desiredStatus":"RUNNING","family":"test","version":"v1","containers":[{"name":"c1","image":"redis","command":["arg1","arg2"],"cpu":10,"memory":20,"links":["db"],"portMappings":[{"containerPort":22,"hostPort":22}],"essential":true,"entryPoint":["bash"],"environment":{"key":"val"},"desiredStatus":"RUNNING"}]}], "messageId": "messageId"}}`)
	expectedContainers := []*api.Container{
		&api.Container{
			Name:    "c1",
			Image:   "redis",
			Command: []string{"arg1", "arg2"},
			Cpu:     10,
			Memory:  20,
			Links:   []string{"db"},
			Ports: []api.PortBinding{
				api.PortBinding{22, 22, ""},
			},
			Essential:     true,
			EntryPoint:    &[]string{"bash"},
			Environment:   map[string]string{"key": "val"},
			DesiredStatus: api.ContainerRunning,
		},
	}
	expectedTasks := []*api.Task{
		&api.Task{
			Arn:           "arn1",
			DesiredStatus: api.TaskRunning,
			Family:        "test",
			Version:       "v1",
			Containers:    expectedContainers,
		},
	}
	response, skip, err = client.parseResponseLine(line)
	tasks := response.Tasks
	if skip {
		t.Error("Wanted to skip a valid response")
	}
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(tasks, expectedTasks) {
		t.Error("Tasks didn't match expected tasks")
	}
}
