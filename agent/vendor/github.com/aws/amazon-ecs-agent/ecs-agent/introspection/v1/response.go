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

package v1

import (
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
)

// AgentMetadataResponse is the schema for the metadata response JSON object.
type AgentMetadataResponse struct {
	Cluster              string  `json:"Cluster"`
	ContainerInstanceArn *string `json:"ContainerInstanceArn"`
	Version              string  `json:"Version"`
}

// ManagedAgentMetadataResponse is the schema for the metadata response JSON object for
// the managed agent platform.
type ManagedAgentMetadataResponse struct {
	Cluster              string  `json:"Cluster"`
	ContainerInstanceArn *string `json:"ContainerInstanceArn"`
}

// TaskResponse is the schema for the task response JSON object.
type TaskResponse struct {
	Arn           string              `json:"Arn"`
	DesiredStatus string              `json:"DesiredStatus,omitempty"`
	KnownStatus   string              `json:"KnownStatus"`
	Family        string              `json:"Family"`
	Version       string              `json:"Version"`
	Containers    []ContainerResponse `json:"Containers"`
}

// TasksResponse is the schema for the tasks response JSON object.
type TasksResponse struct {
	Tasks []*TaskResponse `json:"Tasks"`
}

// ContainerResponse is the schema for the container response JSON object.
type ContainerResponse struct {
	DockerID     string                    `json:"DockerId"`
	DockerName   string                    `json:"DockerName"`
	Name         string                    `json:"Name"`
	Image        string                    `json:"Image"`
	ImageID      string                    `json:"ImageID"`
	CreatedAt    *time.Time                `json:"CreatedAt,omitempty"`
	StartedAt    *time.Time                `json:"StartedAt,omitempty"`
	Ports        []response.PortResponse   `json:"Ports,omitempty"`
	Networks     []response.Network        `json:"Networks,omitempty"`
	Volumes      []response.VolumeResponse `json:"Volumes,omitempty"`
	RestartCount *int                      `json:"RestartCount,omitempty"`
}

// ErrorMultipleTasksFound should be returned when a task cannot be uniquely identified for a given request.
type ErrorMultipleTasksFound struct {
	externalReason string
}

func NewErrorMultipleTasksFound(externalReason string) *ErrorMultipleTasksFound {
	return &ErrorMultipleTasksFound{externalReason: externalReason}
}

func (e *ErrorMultipleTasksFound) Error() string {
	return e.externalReason
}

// StatusCode is the http status code that will appear in the response header when this
// error is thrown.
func (e *ErrorMultipleTasksFound) StatusCode() int {
	return http.StatusBadRequest
}

// MetricName is the metric that will be fired when this error occurs.
func (e *ErrorMultipleTasksFound) MetricName() string {
	return metrics.IntrospectionBadRequest
}

// ErrorNotFound should be returned when the requested metadata cannot be found.
type ErrorNotFound struct {
	externalReason string
}

func NewErrorNotFound(externalReason string) *ErrorNotFound {
	return &ErrorNotFound{externalReason: externalReason}
}

func (e *ErrorNotFound) Error() string {
	return e.externalReason
}

// StatusCode is the http status code that will appear in the response header when this
// error is thrown.
func (e *ErrorNotFound) StatusCode() int {
	return http.StatusNotFound
}

// MetricName is the metric that will be fired when this error occurs.
func (e *ErrorNotFound) MetricName() string {
	return metrics.IntrospectionNotFound
}

// ErrorFetchFailure is a general "catch-all" error to be returned when metadata could not be
// fetched for some reason.
type ErrorFetchFailure struct {
	externalReason string
}

func NewErrorFetchFailure(externalReason string) *ErrorFetchFailure {
	return &ErrorFetchFailure{externalReason: externalReason}
}

func (e *ErrorFetchFailure) Error() string {
	return e.externalReason
}

// StatusCode is the http status code that will appear in the response header when this
// error is thrown.
func (e *ErrorFetchFailure) StatusCode() int {
	return http.StatusInternalServerError
}

// MetricName is the metric that will be fired when this error occurs.
func (e *ErrorFetchFailure) MetricName() string {
	return metrics.IntrospectionFetchFailure
}
