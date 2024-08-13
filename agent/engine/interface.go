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

package engine

import (
	"context"
	"encoding/json"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	dm "github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

// TaskEngine is an interface for the DockerTaskEngine
type TaskEngine interface {
	Init(context.Context) error
	MustInit(context.Context)
	// Disable *must* only be called when this engine will no longer be used
	// (e.g. right before exiting down the process). It will irreversibly stop
	// this task engine from processing new tasks
	Disable()

	// StateChangeEvents will provide information about tasks that have been previously
	// executed. Specifically, it will provide information when they reach
	// running or stopped, as well as providing portbinding and other metadata
	StateChangeEvents() chan statechange.Event
	// SetDataClient sets the data client that is used by the task engine.
	SetDataClient(data.Client)

	// AddTask adds a new task to the task engine and manages its container's
	// lifecycle. If it returns an error, the task was not added.
	AddTask(*apitask.Task)

	// ListTasks lists all the tasks being managed by the TaskEngine.
	ListTasks() ([]*apitask.Task, error)

	// GetTaskByArn gets a managed task, given a task arn.
	GetTaskByArn(string) (*apitask.Task, bool)

	Version() (string, error)

	// LoadState loads the task engine state with data in db.
	LoadState() error
	// SaveState saves all the data in task engine state to db.
	SaveState() error

	GetDaemonManagers() map[string]dm.DaemonManager
	GetDaemonTask(string) *apitask.Task
	SetDaemonTask(string, *apitask.Task)

	json.Marshaler
	json.Unmarshaler
}
