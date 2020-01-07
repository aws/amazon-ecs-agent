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

package firelens

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type firelensResourceJSON struct {
	Cluster                string
	TaskARN                string
	TaskDefinition         string
	EC2InstanceID          string
	ResourceDir            string
	FirelensConfigType     string
	Region                 string
	ECSMetadataEnabled     bool
	ContainersToLogOptions map[string]map[string]string
	ExecutionCredentialsID string
	ExternalConfigType     string
	ExternalConfigValue    string
	TerminalReason         string

	CreatedAt     time.Time
	DesiredStatus *FirelensStatus
	KnownStatus   *FirelensStatus
	AppliedStatus *FirelensStatus
	NetworkMode   string
}

// MarshalJSON marshals a FirelensResource object into bytes of json.
func (firelens *FirelensResource) MarshalJSON() ([]byte, error) {
	if firelens == nil {
		return nil, errors.New("firelens resource is nil")
	}

	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return json.Marshal(firelensResourceJSON{
		Cluster:                firelens.cluster,
		TaskARN:                firelens.taskARN,
		TaskDefinition:         firelens.taskDefinition,
		EC2InstanceID:          firelens.ec2InstanceID,
		ResourceDir:            firelens.resourceDir,
		FirelensConfigType:     firelens.firelensConfigType,
		Region:                 firelens.region,
		ECSMetadataEnabled:     firelens.ecsMetadataEnabled,
		ContainersToLogOptions: firelens.containerToLogOptions,
		ExecutionCredentialsID: firelens.executionCredentialsID,
		ExternalConfigType:     firelens.externalConfigType,
		ExternalConfigValue:    firelens.externalConfigValue,
		TerminalReason:         firelens.terminalReason,
		CreatedAt:              firelens.createdAtUnsafe,
		NetworkMode:            firelens.networkMode,
		DesiredStatus: func() *FirelensStatus {
			desiredStatus := firelens.desiredStatusUnsafe
			s := FirelensStatus(desiredStatus)
			return &s
		}(),
		KnownStatus: func() *FirelensStatus {
			knownStatus := firelens.knownStatusUnsafe
			s := FirelensStatus(knownStatus)
			return &s
		}(),
		AppliedStatus: func() *FirelensStatus {
			appliedStatus := firelens.appliedStatusUnsafe
			s := FirelensStatus(appliedStatus)
			return &s
		}(),
	})
}

// UnmarshalJSON unmarshals bytes of json into a FirelensResource object.
func (firelens *FirelensResource) UnmarshalJSON(b []byte) error {
	if firelens == nil {
		return errors.New("firelens resource is nil")
	}

	temp := firelensResourceJSON{}
	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	firelens.cluster = temp.Cluster
	firelens.taskARN = temp.TaskARN
	firelens.taskDefinition = temp.TaskDefinition
	firelens.ec2InstanceID = temp.EC2InstanceID
	firelens.resourceDir = temp.ResourceDir
	firelens.firelensConfigType = temp.FirelensConfigType
	firelens.region = temp.Region
	firelens.ecsMetadataEnabled = temp.ECSMetadataEnabled
	firelens.containerToLogOptions = temp.ContainersToLogOptions
	firelens.executionCredentialsID = temp.ExecutionCredentialsID
	firelens.externalConfigType = temp.ExternalConfigType
	firelens.externalConfigValue = temp.ExternalConfigValue
	firelens.terminalReason = temp.TerminalReason
	firelens.createdAtUnsafe = temp.CreatedAt
	firelens.desiredStatusUnsafe = resourcestatus.ResourceStatus(*temp.DesiredStatus)
	firelens.knownStatusUnsafe = resourcestatus.ResourceStatus(*temp.KnownStatus)
	firelens.appliedStatusUnsafe = resourcestatus.ResourceStatus(*temp.AppliedStatus)
	firelens.networkMode = temp.NetworkMode

	firelens.initLog()

	return nil
}
