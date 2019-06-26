// +build linux
// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package logrouter

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type logRouterResourceJSON struct {
	Cluster                string
	TaskARN                string
	TaskDefinition         string
	EC2InstanceID          string
	ResourceDir            string
	LogRouterType          string
	ECSMetadataEnabled     bool
	ContainersToLogOptions map[string]map[string]string
	TerminalReason         string

	CreatedAt     time.Time
	DesiredStatus *LogRouterStatus
	KnownStatus   *LogRouterStatus
	AppliedStatus *LogRouterStatus
}

// MarshalJSON marshals logRouter object into bytes of json.
func (logRouter *LogRouterResource) MarshalJSON() ([]byte, error) {
	if logRouter == nil {
		return nil, errors.New("log router resource is nil")
	}

	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return json.Marshal(logRouterResourceJSON{
		Cluster:                logRouter.cluster,
		TaskARN:                logRouter.taskARN,
		TaskDefinition:         logRouter.taskDefinition,
		EC2InstanceID:          logRouter.ec2InstanceID,
		ResourceDir:            logRouter.resourceDir,
		LogRouterType:          logRouter.logRouterType,
		ECSMetadataEnabled:     logRouter.ecsMetadataEnabled,
		ContainersToLogOptions: logRouter.containerToLogOptions,
		TerminalReason:         logRouter.terminalReason,
		CreatedAt:              logRouter.createdAtUnsafe,
		DesiredStatus: func() *LogRouterStatus {
			desiredStatus := logRouter.desiredStatusUnsafe
			s := LogRouterStatus(desiredStatus)
			return &s
		}(),
		KnownStatus: func() *LogRouterStatus {
			knownStatus := logRouter.knownStatusUnsafe
			s := LogRouterStatus(knownStatus)
			return &s
		}(),
		AppliedStatus: func() *LogRouterStatus {
			appliedStatus := logRouter.appliedStatusUnsafe
			s := LogRouterStatus(appliedStatus)
			return &s
		}(),
	})
}

// UnmarshalJSON unmarshals bytes of json into logRouter object.
func (logRouter *LogRouterResource) UnmarshalJSON(b []byte) error {
	if logRouter == nil {
		return errors.New("log router resource is nil")
	}

	temp := logRouterResourceJSON{}
	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	logRouter.cluster = temp.Cluster
	logRouter.taskARN = temp.TaskARN
	logRouter.taskDefinition = temp.TaskDefinition
	logRouter.ec2InstanceID = temp.EC2InstanceID
	logRouter.resourceDir = temp.ResourceDir
	logRouter.logRouterType = temp.LogRouterType
	logRouter.ecsMetadataEnabled = temp.ECSMetadataEnabled
	logRouter.containerToLogOptions = temp.ContainersToLogOptions
	logRouter.terminalReason = temp.TerminalReason
	logRouter.createdAtUnsafe = temp.CreatedAt
	logRouter.desiredStatusUnsafe = resourcestatus.ResourceStatus(*temp.DesiredStatus)
	logRouter.knownStatusUnsafe = resourcestatus.ResourceStatus(*temp.KnownStatus)
	logRouter.appliedStatusUnsafe = resourcestatus.ResourceStatus(*temp.AppliedStatus)

	return nil
}
