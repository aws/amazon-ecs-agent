// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package statemanager_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/stretchr/testify/assert"
)

func TestStateManagerNonexistantDirectory(t *testing.T) {
	cfg := &config.Config{DataDir: "/path/to/directory/that/doesnt/exist"}
	_, err := statemanager.NewStateManager(cfg)
	if err == nil {
		t.Fatal("State manager should not load if the directory doesn't exist")
	}
}

func TestLoadsV1DataCorrectly(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v1", "1", "ecs_agent_data.json"))
	assert.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v1", "1")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewDockerTaskEngineState())
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64

	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = stateManager.Load()
	if err != nil {
		t.Fatal("Error loading state", err)
	}

	if cluster != "test" {
		t.Fatal("Wrong cluster: " + cluster)
	}

	if sequenceNumber != 0 {
		t.Fatal("v1 should give a sequence number of 0")
	}
	tasks, err := taskEngine.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var deadTask *api.Task
	for _, task := range tasks {
		if task.Arn == "arn:aws:ecs:us-west-2:1234567890:task/f44b4fc9-adb0-4f4f-9dff-871512310588" {
			deadTask = task
		}
	}
	if deadTask == nil {
		t.Fatal("Could not find task expected to be in state")
	}
	if deadTask.SentStatus != api.TaskStopped {
		t.Fatal("task dead should be stopped now")
	}
	if deadTask.Containers[0].SentStatus != api.ContainerStopped {
		t.Fatal("container Dead should go to stopped")
	}
	if deadTask.Containers[0].DesiredStatus != api.ContainerStopped {
		t.Fatal("container Dead should go to stopped")
	}
	if deadTask.Containers[0].KnownStatus != api.ContainerStopped {
		t.Fatal("container Dead should go to stopped")
	}
	expected, _ := time.Parse(time.RFC3339, "2015-04-28T17:29:48.129140193Z")
	if deadTask.KnownStatusTime != expected {
		t.Fatal("Time was not correct")
	}
}
