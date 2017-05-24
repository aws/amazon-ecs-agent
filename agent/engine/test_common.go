// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package engine contains the core logic for managing tasks
package engine

import (
	"fmt"
	"reflect"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

func discardEvents(from interface{}) func() {
	done := make(chan bool)

	go func() {
		for {
			ndx, _, _ := reflect.Select([]reflect.SelectCase{
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(from),
				},
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(done),
				},
			})
			if ndx == 1 {
				break
			}
		}
	}()
	return func() {
		done <- true
	}
}

func verifyTaskIsRunning(taskEvents <-chan api.TaskStateChange, testTasks ...*api.Task) error {
	for {
		select {
		case taskEvent := <-taskEvents:
			for i, task := range testTasks {
				if taskEvent.TaskArn != task.Arn {
					continue
				}
				if taskEvent.Status == api.TaskRunning {
					if len(testTasks) == 1 {
						return nil
					}
					testTasks = append(testTasks[:i], testTasks[i+1:]...)
				} else if taskEvent.Status > api.TaskRunning {
					return fmt.Errorf("Task went straight to %s without running, task: %s", taskEvent.Status.String(), task.Arn)
				}
			}
		}
	}
}

func verifyTaskIsStopped(taskEvents <-chan api.TaskStateChange, testTasks ...*api.Task) {
	for {
		select {
		case taskEvent := <-taskEvents:
			for i, task := range testTasks {
				if taskEvent.TaskArn == task.Arn && taskEvent.Status >= api.TaskStopped {
					if len(testTasks) == 1 {
						return
					}
					testTasks = append(testTasks[:i], testTasks[i+1:]...)
				}
			}
		}
	}
}
