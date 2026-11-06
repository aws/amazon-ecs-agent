//go:build !linux
// +build !linux

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

package mpsdaemon

import (
	"context"
	"errors"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
)

// MPSDaemonResource is a stub on non-Linux platforms.
type MPSDaemonResource struct{}

// NewMPSDaemonResource is a stub on non-Linux platforms; MPS is never wired there
// (no container sets MPSConfig), so this only satisfies the cross-platform build.
func NewMPSDaemonResource(ctx context.Context, taskARN string, exec execwrapper.Exec,
	probeCommand string) *MPSDaemonResource {
	return &MPSDaemonResource{}
}

func (r *MPSDaemonResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {}
func (r *MPSDaemonResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) SetKnownStatus(status resourcestatus.ResourceStatus) {}
func (r *MPSDaemonResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) SetCreatedAt(t time.Time) {}
func (r *MPSDaemonResource) GetCreatedAt() time.Time  { return time.Time{} }

// Create fails closed. GPU sharing is rejected for non-Linux platforms before a
// task reaches an instance, so this is unreachable in practice; it exists so the
// package compiles and so a wiring mistake cannot silently skip the gate.
func (r *MPSDaemonResource) Create() error {
	return errors.New("MPS GPU sharing is not supported on this platform")
}
func (r *MPSDaemonResource) Cleanup() error        { return nil }
func (r *MPSDaemonResource) GetName() string       { return ResourceName }
func (r *MPSDaemonResource) DesiredTerminal() bool { return false }
func (r *MPSDaemonResource) KnownCreated() bool    { return false }
func (r *MPSDaemonResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("mps daemon health gate: unsupported platform")
}
func (r *MPSDaemonResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool { return false }
func (r *MPSDaemonResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}
func (r *MPSDaemonResource) StatusString(status resourcestatus.ResourceStatus) string { return "NONE" }
func (r *MPSDaemonResource) GetTerminalReason() string                                { return "" }
func (r *MPSDaemonResource) DependOnTaskNetwork() bool                                { return false }
func (r *MPSDaemonResource) RequiresExecutionRoleCredentials() bool                   { return false }
func (r *MPSDaemonResource) BuildContainerDependency(containerName string,
	satisfied apicontainerstatus.ContainerStatus, dependent resourcestatus.ResourceStatus) {
}
func (r *MPSDaemonResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
func (r *MPSDaemonResource) Initialize(
	cfg *config.Config,
	resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}
func (r *MPSDaemonResource) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }
func (r *MPSDaemonResource) UnmarshalJSON(b []byte) error { return nil }
