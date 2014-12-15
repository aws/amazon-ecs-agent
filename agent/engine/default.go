// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("TaskEngine")

// NewTaskEngine returns a default TaskEngine
func NewTaskEngine() (TaskEngine, error) {
	return InitDockerTaskEngine()
}

// MustTaskEngine tries to create a task engine until it succeeds without
// error. It will print a error message for the first error.
func MustTaskEngine() TaskEngine {
	errorOnce := sync.Once{}

	var taskEngine TaskEngine
	taskEngineConnectBackoff := utils.NewSimpleBackoff(200*time.Millisecond, 2*time.Second, 0.20, 1.5)
	utils.RetryWithBackoff(taskEngineConnectBackoff, func() error {
		var err error
		taskEngine, err = NewTaskEngine()
		if err != nil {
			errorOnce.Do(func() {
				log.Error("Could not connect to docker daemon", "err", err)
			})
		}
		return err
	})

	return taskEngine
}
