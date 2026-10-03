//go:build linux
// +build linux

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
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/mps"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
)

// Retry bounds for the health check. The daemon's systemd unit runs with
// Restart=always and RestartSec=1, so a probe can land in the ~1s window where
// the daemon is restarting and would answer normally a moment later. Retrying
// keeps a task from failing for that transient case.
const (
	probeMaxAttempts     = 3
	probeMinRetryDelay   = 2 * time.Second
	probeMaxRetryDelay   = 4 * time.Second
	probeRetryJitter     = 0.2
	probeRetryMultiplier = 2.0
	defaultGateTimeout   = 10 * time.Second
)

var (
	errDaemonNotAvailable  = errors.New("MPS control daemon is not available on this instance")
	errDaemonNotResponding = errors.New("MPS control daemon is not responding on this instance")
	errGateNotCompleted    = errors.New("MPS control daemon verification did not complete")
)

// MPSDaemonResource verifies the NVIDIA MPS control daemon is functionally
// serving before an MPS task's containers are allowed to be created.
//
// The gate matters because a client that starts while the daemon is down does
// not fail: it bypasses MPS and talks straight to the driver, and the injected
// CUDA_MPS_PINNED_DEVICE_MEM_LIMIT is silently ignored, so the container can
// consume the whole GPU and starve its co-tenants with nothing logged.
type MPSDaemonResource struct {
	ctx     context.Context
	taskARN string
	// probeCommand is the control command sent to the control utility. Defaults
	// to mps.ProbeCommand.
	probeCommand string
	exec         execwrapper.Exec
	osWrapper    oswrapper.OS
	newBackoff   func() retry.Backoff
	gateTimeout  time.Duration

	createdAt           time.Time
	desiredStatusUnsafe resourcestatus.ResourceStatus
	knownStatusUnsafe   resourcestatus.ResourceStatus
	appliedStatus       resourcestatus.ResourceStatus
	statusToTransitions map[resourcestatus.ResourceStatus]func() error
	terminalReason      string
	terminalReasonOnce  sync.Once
	lock                sync.RWMutex
}

// NewMPSDaemonResource returns a new health-gate resource for a task. An empty
// probeCommand selects mps.ProbeCommand.
func NewMPSDaemonResource(ctx context.Context, taskARN string, exec execwrapper.Exec,
	probeCommand string) *MPSDaemonResource {
	r := &MPSDaemonResource{
		ctx:          ctx,
		taskARN:      taskARN,
		exec:         exec,
		probeCommand: probeCommand,
	}
	r.applyDefaults()
	r.initStatusToTransitionFunction()
	return r
}

// applyDefaults fills in the dependencies and settings that are not carried
// across a marshal/unmarshal round trip. Callers must hold the write lock.
func (r *MPSDaemonResource) applyDefaults() {
	if r.ctx == nil {
		r.ctx = context.Background()
	}
	if r.probeCommand == "" {
		r.probeCommand = mps.ProbeCommand
	}
	if r.exec == nil {
		r.exec = execwrapper.NewExec()
	}
	if r.osWrapper == nil {
		r.osWrapper = oswrapper.NewOS()
	}
	if r.gateTimeout == 0 {
		r.gateTimeout = defaultGateTimeout
	}
	if r.newBackoff == nil {
		r.newBackoff = func() retry.Backoff {
			return retry.NewExponentialBackoff(probeMinRetryDelay, probeMaxRetryDelay,
				probeRetryJitter, probeRetryMultiplier)
		}
	}
}

func (r *MPSDaemonResource) initStatusToTransitionFunction() {
	r.statusToTransitions = map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(MPSDaemonCreated): r.Create,
	}
}

// probeDeps is a snapshot of the injected dependencies, taken under the lock so
// a concurrent Initialize cannot be observed mid-write.
type probeDeps struct {
	exec         execwrapper.Exec
	osWrapper    oswrapper.OS
	probeCommand string
	newBackoff   func() retry.Backoff
	gateTimeout  time.Duration
	parent       context.Context
}

func (r *MPSDaemonResource) snapshotDeps() probeDeps {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return probeDeps{
		exec:         r.exec,
		osWrapper:    r.osWrapper,
		probeCommand: r.probeCommand,
		newBackoff:   r.newBackoff,
		gateTimeout:  r.gateTimeout,
		parent:       r.ctx,
	}
}

// Create is the resource's CREATED-transition function and nothing is created. For this
// health gate, reaching CREATED means an attempt confirmed the MPS daemon is serving.
// It runs a bounded retry, returning nil as soon as an attempt finds the daemon serving.
// Any other outcome stops the task with a static reason and logs the evidence.
func (r *MPSDaemonResource) Create() error {
	// Only tests build the resource directly; otherwise the constructor or
	// Initialize already set these.
	r.lock.Lock()
	r.applyDefaults()
	r.lock.Unlock()

	deps := r.snapshotDeps()
	ctx, cancel := context.WithTimeout(deps.parent, deps.gateTimeout)
	defer cancel()

	attempt := 0
	err := retry.RetryNWithBackoffCtx(ctx, deps.newBackoff(), probeMaxAttempts,
		func() error {
			attempt++
			return r.checkOnce(attempt, deps)
		})

	// RetryNWithBackoffCtx reports nil when the context is already done, having
	// never run the check. Nothing was verified, so fail closed: a gate that
	// passes without probing would let the injected memory cap be ignored, which
	// is the failure this resource exists to prevent.
	if attempt == 0 {
		return r.blockTask(errGateNotCompleted, "context was already done, no probe ran")
	}
	if err != nil {
		// The parent going down means the agent is stopping or the task was
		// stopped. That is not the daemon's fault, so do not report it as such.
		if deps.parent.Err() != nil {
			return r.blockTask(errGateNotCompleted, "abandoned while retrying")
		}
		return r.blockTask(err, fmt.Sprintf("no success in %d attempts", attempt))
	}
	return nil
}

// blockTask records the customer-facing reason and logs why.
func (r *MPSDaemonResource) blockTask(cause error, detail string) error {
	r.setTerminalReason(cause.Error())
	logger.Error("MPS daemon health gate: blocking task", logger.Fields{
		field.TaskARN: r.taskARN,
		"detail":      detail,
		field.Error:   cause,
	})
	return cause
}

// checkOnce runs one attempt: pipe-directory usability, then the probe. It does
// not set the terminal reason; Create does that once the retry is spent, so an
// attempt that later succeeds leaves no reason behind.
func (r *MPSDaemonResource) checkOnce(attempt int, deps probeDeps) error {
	// An unusable pipe directory means the agent cannot reach the daemon at all.
	// Usually that is the daemon never having started, but it also covers the agent
	// having started without the bind mount: ecs-init only mounts the directory if
	// it already exists, so an agent that came up before the daemon stays blind
	// until it is restarted. Stopping the daemon does not produce this state, since
	// it leaves the directory behind and removes only its sockets. Retrying cannot
	// help in either case, so this is non-retriable.
	fi, statErr := deps.osWrapper.Stat(mps.PipeDirectory)
	if statErr != nil || !fi.IsDir() {
		logger.Warn("MPS daemon health gate: pipe directory unusable", logger.Fields{
			field.TaskARN: r.taskARN,
			"attempt":     attempt,
			"path":        mps.PipeDirectory,
			field.Error:   statErr,
		})
		return apierrors.NewRetriableError(apierrors.NewRetriable(false), errDaemonNotAvailable)
	}

	res := mps.ProbeControlDaemon(deps.exec, deps.probeCommand)
	logger.Info("MPS daemon health gate: probe complete", logger.Fields{
		field.TaskARN: r.taskARN,
		"attempt":     attempt,
		"command":     deps.probeCommand,
		"exitCode":    res.ExitCode,
		"stdout":      res.Stdout,
		"latencyMs":   res.Latency.Milliseconds(),
		"timedOut":    res.TimedOut,
	})
	if res.Err != nil {
		logger.Warn("MPS daemon health gate: attempt failed", logger.Fields{
			field.TaskARN: r.taskARN,
			"attempt":     attempt,
			"timedOut":    res.TimedOut,
			field.Error:   res.Err,
		})
		return errDaemonNotResponding
	}
	return nil
}

// Cleanup is a no-op: the health gate owns no host state.
func (r *MPSDaemonResource) Cleanup() error {
	return nil
}

func (r *MPSDaemonResource) setTerminalReason(reason string) {
	r.terminalReasonOnce.Do(func() {
		r.lock.Lock()
		defer r.lock.Unlock()
		r.terminalReason = reason
	})
}

// GetTerminalReason returns why the resource failed to provision.
func (r *MPSDaemonResource) GetTerminalReason() string {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.terminalReason
}

// GetName returns the unique name of the resource.
func (r *MPSDaemonResource) GetName() string { return ResourceName }

// SetDesiredStatus sets the desired status of the resource.
func (r *MPSDaemonResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.desiredStatusUnsafe = status
}

// GetDesiredStatus gets the desired status of the resource.
func (r *MPSDaemonResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.desiredStatusUnsafe
}

// SetKnownStatus sets the known status of the resource.
func (r *MPSDaemonResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.knownStatusUnsafe = status
	r.updateAppliedStatusUnsafe(status)
}

func (r *MPSDaemonResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if r.appliedStatus == resourcestatus.ResourceStatus(MPSDaemonStatusNone) {
		return
	}
	if r.appliedStatus <= knownStatus {
		r.appliedStatus = resourcestatus.ResourceStatus(MPSDaemonStatusNone)
	}
}

// GetKnownStatus gets the known status of the resource.
func (r *MPSDaemonResource) GetKnownStatus() resourcestatus.ResourceStatus {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.knownStatusUnsafe
}

// SetCreatedAt sets the timestamp for the resource's creation time.
func (r *MPSDaemonResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.createdAt = createdAt
}

// GetCreatedAt gets the timestamp for the resource's creation time.
func (r *MPSDaemonResource) GetCreatedAt() time.Time {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.createdAt
}

// DesiredTerminal returns true if the resource's desired state is terminal.
func (r *MPSDaemonResource) DesiredTerminal() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.desiredStatusUnsafe == resourcestatus.ResourceStatus(MPSDaemonRemoved)
}

// KnownCreated returns true if the daemon resource's known status is CREATED
func (r *MPSDaemonResource) KnownCreated() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.knownStatusUnsafe == resourcestatus.ResourceStatus(MPSDaemonCreated)
}

// TerminalStatus returns the last transition state of the resource.
func (r *MPSDaemonResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(MPSDaemonRemoved)
}

// NextKnownState returns the resource's next state.
func (r *MPSDaemonResource) NextKnownState() resourcestatus.ResourceStatus {
	return r.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status.
func (r *MPSDaemonResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := r.statusToTransitions[nextState]
	if !ok {
		return fmt.Errorf("resource [%s]: transition to %s impossible", r.GetName(),
			r.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready".
func (r *MPSDaemonResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(MPSDaemonCreated)
}

// SetAppliedStatus sets the applied status of the resource and returns whether
// the resource is already in a transition.
func (r *MPSDaemonResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.appliedStatus != resourcestatus.ResourceStatus(MPSDaemonStatusNone) {
		return false
	}
	r.appliedStatus = status
	return true
}

// GetAppliedStatus gets the applied status of the resource.
func (r *MPSDaemonResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.appliedStatus
}

// StatusString returns the string form of a resource status.
func (r *MPSDaemonResource) StatusString(status resourcestatus.ResourceStatus) string {
	return MPSDaemonStatus(status).String()
}

// DependOnTaskNetwork reports whether the resource needs task network setup.
func (r *MPSDaemonResource) DependOnTaskNetwork() bool { return false }

// RequiresExecutionRoleCredentials reports whether the resource needs execution
// role credentials.
func (r *MPSDaemonResource) RequiresExecutionRoleCredentials() bool { return false }

func (r *MPSDaemonResource) BuildContainerDependency(containerName string,
	satisfied apicontainerstatus.ContainerStatus, dependent resourcestatus.ResourceStatus) {
}

// GetContainerDependencies returns the resource's dependent containers; this
// gate has none of its own.
func (r *MPSDaemonResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}

// Initialize re-establishes the dependencies that do not survive a marshal
// round trip. It runs both for a freshly received task and for one restored
// from disk after an agent restart.
//
// The known status is deliberately left as restored rather than reset. A
// resource that had already reached CREATED belongs to a task whose containers
// are running, and re-probing there could stop a healthy task because the daemon
// happened to be restarting at that moment. A task restored before the gate ran
// comes back as NONE and probes normally.
func (r *MPSDaemonResource) Initialize(
	cfg *config.Config,
	resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if resourceFields != nil && resourceFields.Ctx != nil {
		r.ctx = resourceFields.Ctx
	}
	r.applyDefaults()
	r.initStatusToTransitionFunction()
}

// mpsDaemonResourceJSON is the marshalling shadow struct.
type mpsDaemonResourceJSON struct {
	TaskARN string `json:"taskARN"`
	// probeCommand is deliberately not serialized: it is a package constant that
	// only tests override, so persisting it would pin a stale value across an
	// agent upgrade. applyDefaults supplies the current default on restore.
	CreatedAt     *time.Time       `json:"createdAt,omitempty"`
	DesiredStatus *MPSDaemonStatus `json:"desiredStatus"`
	KnownStatus   *MPSDaemonStatus `json:"knownStatus"`
}

// MarshalJSON serializes the resource.
func (r *MPSDaemonResource) MarshalJSON() ([]byte, error) {
	if r == nil {
		return nil, errors.New("mpsdaemon resource is nil")
	}
	desired := MPSDaemonStatus(r.GetDesiredStatus())
	known := MPSDaemonStatus(r.GetKnownStatus())
	createdAt := r.GetCreatedAt()
	r.lock.RLock()
	taskARN := r.taskARN
	r.lock.RUnlock()
	return json.Marshal(mpsDaemonResourceJSON{
		TaskARN:       taskARN,
		CreatedAt:     &createdAt,
		DesiredStatus: &desired,
		KnownStatus:   &known,
	})
}

// UnmarshalJSON deserializes the resource. Dependencies are not part of the
// serialized form; Initialize re-establishes them.
func (r *MPSDaemonResource) UnmarshalJSON(b []byte) error {
	temp := mpsDaemonResourceJSON{}
	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}
	r.lock.Lock()
	r.taskARN = temp.TaskARN
	r.lock.Unlock()
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		r.SetCreatedAt(*temp.CreatedAt)
	}
	if temp.DesiredStatus != nil {
		r.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		r.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	return nil
}
