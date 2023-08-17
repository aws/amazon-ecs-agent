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

package handler

import (
	"strconv"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
)

// taskComparer implements the TaskComparer interface defined in ecs-agent module.
type taskComparer struct {
	taskEngine engine.TaskEngine
}

// NewTaskComparer creates a new taskComparer.
func NewTaskComparer(taskEngine engine.TaskEngine) *taskComparer {
	return &taskComparer{
		taskEngine: taskEngine,
	}
}

// sequenceNumberAccessor implements the SequenceNumberAccessor interface defined in ecs-agent module.
type sequenceNumberAccessor struct {
	latestSeqNumberTaskManifest *int64
	dataClient                  data.Client
}

// NewSequenceNumberAccessor creates a new NewSequenceNumberAccessor.
func NewSequenceNumberAccessor(latestSeqNumberTaskManifest *int64, dataClient data.Client) *sequenceNumberAccessor {
	return &sequenceNumberAccessor{
		latestSeqNumberTaskManifest: latestSeqNumberTaskManifest,
		dataClient:                  dataClient,
	}
}

func (tc *taskComparer) CompareRunningTasksOnInstanceWithManifest(
	message *ecsacs.TaskManifestMessage) ([]*ecsacs.TaskIdentifier, error) {
	runningTasksOnInstance, err := tc.taskEngine.ListTasks()
	if err != nil {
		return nil, err
	}

	tasksToBeKilled := make([]*ecsacs.TaskIdentifier, 0)
	for _, runningTask := range runningTasksOnInstance {
		// For every task running on the instance check if the task is present in the task manifest with
		// the DesiredStatus of running. If not, add them to the list of tasks that need to be stopped.
		if runningTask.GetDesiredStatus() == apitaskstatus.TaskRunning {
			taskPresent := false
			for _, manifestTask := range message.Tasks {
				if *manifestTask.TaskArn == runningTask.Arn &&
					*manifestTask.DesiredStatus == apitaskstatus.TaskRunningString {
					// Task present, does not need to be stopped.
					taskPresent = true
					break
				}
			}
			if !taskPresent {
				tasksToBeKilled = append(tasksToBeKilled, &ecsacs.TaskIdentifier{
					DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
					TaskArn:        aws.String(runningTask.Arn),
					TaskClusterArn: message.ClusterArn,
				})
			}
		}
	}
	return tasksToBeKilled, nil
}

func (sna *sequenceNumberAccessor) GetLatestSequenceNumber() int64 {
	return *sna.latestSeqNumberTaskManifest
}

func (sna *sequenceNumberAccessor) SetLatestSequenceNumber(message *ecsacs.TaskManifestMessage) error {
	*sna.latestSeqNumberTaskManifest = *message.Timeline

	// Save the new sequence number to disk.
	err := sna.dataClient.SaveMetadata(data.TaskManifestSeqNumKey, strconv.FormatInt(*message.Timeline, 10))
	if err != nil {
		return err
	}
	return nil
}
