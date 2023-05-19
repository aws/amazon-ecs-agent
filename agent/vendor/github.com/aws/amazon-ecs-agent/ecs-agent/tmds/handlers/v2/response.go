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
package v2

import (
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
)

// TaskResponse defines the schema for the task response JSON object
type TaskResponse struct {
	Cluster               string              `json:"Cluster"`
	TaskARN               string              `json:"TaskARN"`
	Family                string              `json:"Family"`
	Revision              string              `json:"Revision"`
	DesiredStatus         string              `json:"DesiredStatus,omitempty"`
	KnownStatus           string              `json:"KnownStatus"`
	Containers            []ContainerResponse `json:"Containers,omitempty"`
	Limits                *LimitsResponse     `json:"Limits,omitempty"`
	PullStartedAt         *time.Time          `json:"PullStartedAt,omitempty"`
	PullStoppedAt         *time.Time          `json:"PullStoppedAt,omitempty"`
	ExecutionStoppedAt    *time.Time          `json:"ExecutionStoppedAt,omitempty"`
	AvailabilityZone      string              `json:"AvailabilityZone,omitempty"`
	TaskTags              map[string]string   `json:"TaskTags,omitempty"`
	ContainerInstanceTags map[string]string   `json:"ContainerInstanceTags,omitempty"`
	LaunchType            string              `json:"LaunchType,omitempty"`
	Errors                []ErrorResponse     `json:"Errors,omitempty"`
}

// ContainerResponse defines the schema for the container response
// JSON object
type ContainerResponse struct {
	ID            string                    `json:"DockerId"`
	Name          string                    `json:"Name"`
	DockerName    string                    `json:"DockerName"`
	Image         string                    `json:"Image"`
	ImageID       string                    `json:"ImageID"`
	Ports         []response.PortResponse   `json:"Ports,omitempty"`
	Labels        map[string]string         `json:"Labels,omitempty"`
	DesiredStatus string                    `json:"DesiredStatus"`
	KnownStatus   string                    `json:"KnownStatus"`
	ExitCode      *int                      `json:"ExitCode,omitempty"`
	Limits        LimitsResponse            `json:"Limits"`
	CreatedAt     *time.Time                `json:"CreatedAt,omitempty"`
	StartedAt     *time.Time                `json:"StartedAt,omitempty"`
	FinishedAt    *time.Time                `json:"FinishedAt,omitempty"`
	Type          string                    `json:"Type"`
	Networks      []response.Network        `json:"Networks,omitempty"`
	Health        *HealthStatus             `json:"Health,omitempty"`
	Volumes       []response.VolumeResponse `json:"Volumes,omitempty"`
	LogDriver     string                    `json:"LogDriver,omitempty"`
	LogOptions    map[string]string         `json:"LogOptions,omitempty"`
	ContainerARN  string                    `json:"ContainerARN,omitempty"`
}

// Container health status
type HealthStatus struct {
	// Status is the container health status
	Status string `json:"status,omitempty"`
	// Since is the timestamp when container health status changed
	Since *time.Time `json:"statusSince,omitempty"`
	// ExitCode is the exitcode of health check if failed
	ExitCode int `json:"exitCode,omitempty"`
	// Output is the output of health check
	Output string `json:"output,omitempty"`
}

// LimitsResponse defines the schema for task/cpu limits response
// JSON object
type LimitsResponse struct {
	CPU    *float64 `json:"CPU,omitempty"`
	Memory *int64   `json:"Memory,omitempty"`
}

// ErrorResponse defined the schema for error response
// JSON object
type ErrorResponse struct {
	ErrorField   string `json:"ErrorField,omitempty"`
	ErrorCode    string `json:"ErrorCode,omitempty"`
	ErrorMessage string `json:"ErrorMessage,omitempty"`
	StatusCode   int    `json:"StatusCode,omitempty"`
	RequestId    string `json:"RequestId,omitempty"`
	ResourceARN  string `json:"ResourceARN,omitempty"`
}
