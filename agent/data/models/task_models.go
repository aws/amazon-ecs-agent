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

//lint:file-ignore U1000 Ignore unused fields as some of them are only used by Fargate

package models

import (
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	nlappmesh "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
)

// Task_1_0_0 is "the original model" before model transformer is created.
type Task_1_0_0 struct {
	Arn                                    string
	id                                     string
	Overrides                              task.TaskOverrides `json:"-"`
	Family                                 string
	Version                                string
	ServiceName                            string
	Containers                             []*apicontainer.Container
	Associations                           []task.Association        `json:"associations"`
	ResourcesMapUnsafe                     resourcetype.ResourcesMap `json:"resources"`
	Volumes                                []task.TaskVolume         `json:"volumes"`
	CPU                                    float64                   `json:"Cpu,omitempty"`
	Memory                                 int64                     `json:"Memory,omitempty"`
	DesiredStatusUnsafe                    apitaskstatus.TaskStatus  `json:"DesiredStatus"`
	KnownStatusUnsafe                      apitaskstatus.TaskStatus  `json:"KnownStatus"`
	KnownStatusTimeUnsafe                  time.Time                 `json:"KnownTime"`
	PullStartedAtUnsafe                    time.Time                 `json:"PullStartedAt"`
	PullStoppedAtUnsafe                    time.Time                 `json:"PullStoppedAt"`
	ExecutionStoppedAtUnsafe               time.Time                 `json:"ExecutionStoppedAt"`
	SentStatusUnsafe                       apitaskstatus.TaskStatus  `json:"SentStatus"`
	ExecutionCredentialsID                 string                    `json:"executionCredentialsID"`
	credentialsID                          string
	credentialsRelativeURIUnsafe           string
	ENIs                                   task.TaskENIs `json:"ENI"`
	AppMesh                                *nlappmesh.AppMesh
	MemoryCPULimitsEnabled                 bool                `json:"MemoryCPULimitsEnabled,omitempty"`
	PlatformFields                         task.PlatformFields `json:"PlatformFields,omitempty"`
	terminalReason                         string
	terminalReasonOnce                     sync.Once
	PIDMode                                string `json:"PidMode,omitempty"`
	IPCMode                                string `json:"IpcMode,omitempty"`
	NvidiaRuntime                          string `json:"NvidiaRuntime,omitempty"`
	LocalIPAddressUnsafe                   string `json:"LocalIPAddress,omitempty"`
	LaunchType                             string `json:"LaunchType,omitempty"`
	lock                                   sync.RWMutex
	setIdOnce                              sync.Once
	ServiceConnectConfig                   *serviceconnect.Config `json:"ServiceConnectConfig,omitempty"`
	ServiceConnectConnectionDrainingUnsafe bool                   `json:"ServiceConnectConnectionDraining,omitempty"`
	NetworkMode                            string                 `json:"NetworkMode,omitempty"`
	IsInternal                             bool                   `json:"IsInternal,omitempty"`
}

// Task_1_x_0 is an example new model with breaking change. Latest Task_1_x_0 should be the same as current Task model.
// TODO: update this model when introducing first actual transformation function
type Task_1_x_0 struct {
	Arn                                    string
	id                                     string
	Overrides                              task.TaskOverrides `json:"-"`
	Family                                 string
	Version                                string
	ServiceName                            string
	Containers                             []*apicontainer.Container
	Associations                           []task.Association        `json:"associations"`
	ResourcesMapUnsafe                     resourcetype.ResourcesMap `json:"resources"`
	Volumes                                []task.TaskVolume         `json:"volumes"`
	CPU                                    float64                   `json:"Cpu,omitempty"`
	Memory                                 int64                     `json:"Memory,omitempty"`
	DesiredStatusUnsafe                    apitaskstatus.TaskStatus  `json:"DesiredStatus"`
	KnownStatusUnsafe                      apitaskstatus.TaskStatus  `json:"KnownStatus"`
	KnownStatusTimeUnsafe                  time.Time                 `json:"KnownTime"`
	PullStartedAtUnsafe                    time.Time                 `json:"PullStartedAt"`
	PullStoppedAtUnsafe                    time.Time                 `json:"PullStoppedAt"`
	ExecutionStoppedAtUnsafe               time.Time                 `json:"ExecutionStoppedAt"`
	SentStatusUnsafe                       apitaskstatus.TaskStatus  `json:"SentStatus"`
	ExecutionCredentialsID                 string                    `json:"executionCredentialsID"`
	credentialsID                          string
	credentialsRelativeURIUnsafe           string
	NetworkInterfaces                      task.TaskENIs `json:"NetworkInterfaces"`
	AppMesh                                *nlappmesh.AppMesh
	MemoryCPULimitsEnabled                 bool                `json:"MemoryCPULimitsEnabled,omitempty"`
	PlatformFields                         task.PlatformFields `json:"PlatformFields,omitempty"`
	terminalReason                         string
	terminalReasonOnce                     sync.Once
	PIDMode                                string `json:"PidMode,omitempty"`
	IPCMode                                string `json:"IpcMode,omitempty"`
	NvidiaRuntime                          string `json:"NvidiaRuntime,omitempty"`
	LocalIPAddressUnsafe                   string `json:"LocalIPAddress,omitempty"`
	LaunchType                             string `json:"LaunchType,omitempty"`
	lock                                   sync.RWMutex
	setIdOnce                              sync.Once
	ServiceConnectConfig                   *serviceconnect.Config `json:"ServiceConnectConfig,omitempty"`
	ServiceConnectConnectionDrainingUnsafe bool                   `json:"ServiceConnectConnectionDraining,omitempty"`
	NetworkMode                            string                 `json:"NetworkMode,omitempty"`
	IsInternal                             bool                   `json:"IsInternal,omitempty"`
}
