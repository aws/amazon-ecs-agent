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

package statemanager_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	engine_testutils "github.com/aws/amazon-ecs-agent/agent/engine/testutils"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
)

func TestStateManager(t *testing.T) {
	tmpDir, err := ioutil.TempDir("/tmp", "ecs_statemanager_test")
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	cfg := &config.Config{DataDir: tmpDir}
	manager, err := statemanager.NewStateManager(cfg)
	if err != nil {
		t.Fatal("Error loading manager", err)
	}

	err = manager.Load()
	if err != nil {
		t.Error("Expected loading a non-existant file to not be an error")
	}

	// Now let's make some state to save
	containerInstanceArn := ""
	taskEngine := engine.NewTaskEngine(&config.Config{}, false)

	manager, err = statemanager.NewStateManager(cfg, statemanager.AddSaveable("TaskEngine", taskEngine), statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn))
	if err != nil {
		t.Fatal(err)
	}

	containerInstanceArn = "containerInstanceArn"

	testTask := &api.Task{Arn: "test-arn"}
	taskEngine.(*engine.DockerTaskEngine).State().AddTask(testTask)

	err = manager.Save()
	if err != nil {
		t.Fatal("Error saving state", err)
	}

	// Now make sure we can load that state sanely
	loadedTaskEngine := engine.NewTaskEngine(&config.Config{}, false)
	var loadedContainerInstanceArn string

	manager, err = statemanager.NewStateManager(cfg, statemanager.AddSaveable("TaskEngine", &loadedTaskEngine), statemanager.AddSaveable("ContainerInstanceArn", &loadedContainerInstanceArn))
	if err != nil {
		t.Fatal(err)
	}

	err = manager.Load()
	if err != nil {
		t.Fatal("Error loading state", err)
	}

	if loadedContainerInstanceArn != containerInstanceArn {
		t.Error("Did not load containerInstanceArn correctly; got ", loadedContainerInstanceArn, " instead of ", containerInstanceArn)
	}

	if !engine_testutils.DockerTaskEnginesEqual(loadedTaskEngine.(*engine.DockerTaskEngine), (taskEngine.(*engine.DockerTaskEngine))) {
		t.Error("Did not load taskEngine correctly")
	}

	// I'd rather double check .Equal there; let's make sure ListTasks agrees.
	tasks, err := loadedTaskEngine.ListTasks()
	if err != nil {
		t.Error("Error listing tasks", err)
	}
	if len(tasks) != 1 {
		t.Error("Should have a task!")
	} else {
		if tasks[0].Arn != "test-arn" {
			t.Error("Wrong arn, expected test-arn but got ", tasks[0].Arn)
		}
	}
}

func TestStateManagerNonexistantDirectory(t *testing.T) {
	cfg := &config.Config{DataDir: "/path/to/directory/that/doesnt/exist"}
	_, err := statemanager.NewStateManager(cfg)
	if err == nil {
		t.Fatal("State manager should not load if the directory doesn't exist")
	}
}

func TestLoadsV1DataCorrectly(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v1", "1")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, false)
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
