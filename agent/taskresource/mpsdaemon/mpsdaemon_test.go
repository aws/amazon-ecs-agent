//go:build linux && unit
// +build linux,unit

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
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
	mock_execwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/mps"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MPSDaemonResource must satisfy the task resource contract.
var _ taskresource.TaskResource = (*MPSDaemonResource)(nil)

const testTaskARN = "arn:aws:ecs:us-west-2:123456789012:task/cluster/abc123"

// dirInfo is a stub os.FileInfo that reports itself as a directory.
type dirInfo struct{ os.FileInfo }

func (dirInfo) IsDir() bool { return true }

// fileInfo is a stub os.FileInfo that reports itself as a regular file.
type fileInfo struct{ os.FileInfo }

func (fileInfo) IsDir() bool { return false }

// newTestResource builds a resource with a stub filesystem and a retry schedule
// short enough that exhausting the budget does not slow the test. statErr nil
// means the pipe directory is present.
// statOS adapts a stat function to oswrapper.OS. Only Stat is exercised by the
// gate, so the embedded interface is left nil; any other call would panic, which
// is the desired signal if the gate ever grows a new dependency on os.
type statOS struct {
	oswrapper.OS
	stat func(string) (os.FileInfo, error)
}

func (s statOS) Stat(path string) (os.FileInfo, error) { return s.stat(path) }

// dirStat reports the pipe directory as present, or statErr if non-nil.
func dirStat(statErr error) func(string) (os.FileInfo, error) {
	return func(string) (os.FileInfo, error) {
		if statErr != nil {
			return nil, statErr
		}
		return dirInfo{}, nil
	}
}

func newTestResource(exec execwrapper.Exec, statErr error) *MPSDaemonResource {
	return newTestResourceWithStat(exec, dirStat(statErr))
}

// newTestResourceWithStat lets a test supply its own Stat behaviour for the cases
// that need to count or vary the call.
func newTestResourceWithStat(exec execwrapper.Exec, stat func(string) (os.FileInfo, error)) *MPSDaemonResource {
	r := NewMPSDaemonResource(context.Background(), testTaskARN, exec, mps.ProbeCommand)
	r.osWrapper = statOS{stat: stat}
	r.newBackoff = func() retry.Backoff {
		return retry.NewExponentialBackoff(time.Millisecond, 2*time.Millisecond, 0, 1)
	}
	return r
}

// expectProbe queues the call sequence ProbeControlDaemon makes for one attempt.
func expectProbe(mockExec *mock_execwrapper.MockExec, mockCmd *mock_execwrapper.MockCmd,
	out []byte, err error) {
	mockExec.EXPECT().NewExecContextWithTimeout(gomock.Any(), mps.ProbeTimeout).
		DoAndReturn(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, d)
		})
	mockExec.EXPECT().CommandContext(gomock.Any(), mps.ControlBinary).Return(mockCmd)
	// SetEnv carries CUDA_MPS_PIPE_DIRECTORY to the control utility.
	mockCmd.EXPECT().SetEnv(gomock.Any())
	mockCmd.EXPECT().SetIOStreams(gomock.Any(), gomock.Any(), gomock.Any())
	mockCmd.EXPECT().CombinedOutput().Return(out, err)
	if err != nil {
		mockExec.EXPECT().ConvertToExitError(err).Return(&exec.ExitError{}, true)
		mockExec.EXPECT().GetExitCode(gomock.Any()).Return(1)
	}
}

func TestCreatePassesWhenDaemonServing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	expectProbe(mockExec, mockCmd, []byte("100.0\n"), nil)

	r := newTestResource(mockExec, nil)

	assert.NoError(t, r.Create(), "a serving daemon must let the gate pass")
	assert.Empty(t, r.GetTerminalReason(), "a passing gate must record no terminal reason")
}

// A missing pipe directory means the daemon never started. The probe must be
// skipped entirely so the task reason names that case specifically.
func TestCreateBlocksWhenPipeDirectoryMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// No probe calls are queued, so any exec use fails the test.
	mockExec := mock_execwrapper.NewMockExec(ctrl)

	statCalls := 0
	r := newTestResourceWithStat(mockExec, func(string) (os.FileInfo, error) {
		statCalls++
		return nil, errors.New("no such file or directory")
	})

	err := r.Create()

	require.Error(t, err, "a missing pipe directory must block the task")
	// The error is wrapped non-retriable to stop the retry loop, and
	// DefaultRetriableError has no Unwrap, so compare the message rather than the
	// identity. The message is what the customer sees.
	assert.Equal(t, errDaemonNotAvailable.Error(), err.Error())
	assert.Equal(t, errDaemonNotAvailable.Error(), r.GetTerminalReason(),
		"the terminal reason is what surfaces as the task's stopped reason")
	assert.NotContains(t, r.GetTerminalReason(), mps.PipeDirectory,
		"host paths must not reach the customer-facing reason")
	assert.Equal(t, 1, statCalls,
		"the daemon creates the pipe directory and nothing removes it, so this cannot "+
			"clear on retry and must fail fast")
}

func TestCreateBlocksWhenDaemonNotServing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	probeErr := errors.New("Cannot find MPS control daemon process")
	for i := 0; i < probeMaxAttempts; i++ {
		expectProbe(mockExec, mockCmd, []byte(""), probeErr)
	}

	r := newTestResource(mockExec, nil)

	err := r.Create()

	require.Error(t, err, "a daemon that is not serving must block the task")
	assert.Equal(t, errDaemonNotResponding, err)
	assert.Equal(t, errDaemonNotResponding.Error(), r.GetTerminalReason())
}

// The retry exists for this case: the daemon is mid-restart on the first attempt
// and answering by the next one. systemd restarts it with RestartSec=1, so a
// task must not fail for a window that closes on its own.
func TestCreateRecoversWhenDaemonRestarts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	expectProbe(mockExec, mockCmd, []byte(""), errors.New("Cannot find MPS control daemon process"))
	expectProbe(mockExec, mockCmd, []byte("100.0\n"), nil)

	r := newTestResource(mockExec, nil)

	assert.NoError(t, r.Create(), "the gate must ride over a daemon restart")
	assert.Empty(t, r.GetTerminalReason(),
		"an attempt that later succeeds must leave no terminal reason behind")
}

// A wedged daemon accepts the connection and never replies. The reason must say
// so, because it is a different operator problem from a daemon that is absent.
func TestCreateBlocksWhenDaemonWedged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	for i := 0; i < probeMaxAttempts; i++ {
		// An already expired context makes ctx.Err() report DeadlineExceeded.
		mockExec.EXPECT().NewExecContextWithTimeout(gomock.Any(), mps.ProbeTimeout).
			DoAndReturn(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
				return context.WithTimeout(parent, 0)
			})
		mockExec.EXPECT().CommandContext(gomock.Any(), mps.ControlBinary).Return(mockCmd)
		mockCmd.EXPECT().SetEnv(gomock.Any())
		mockCmd.EXPECT().SetIOStreams(gomock.Any(), gomock.Any(), gomock.Any())
		killErr := errors.New("signal: killed")
		mockCmd.EXPECT().CombinedOutput().Return([]byte{}, killErr)
		mockExec.EXPECT().ConvertToExitError(killErr).Return(nil, false)
	}

	r := newTestResource(mockExec, nil)

	err := r.Create()

	require.Error(t, err, "a wedged daemon must block the task")
	assert.Equal(t, errDaemonNotResponding, err,
		"a wedged daemon is reported as not responding; the timeout detail goes to the log")
}

// An already-cancelled context makes the retry helper return before running the
// check at all. The gate must NOT pass in that case: verifying nothing and
// reporting success would let the injected memory cap be silently ignored.
func TestCreateFailsClosedWhenContextAlreadyCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// No probe is queued: any exec use would be an unexpected call.
	mockExec := mock_execwrapper.NewMockExec(ctrl)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	statCalls := 0
	r := newTestResourceWithStat(mockExec, func(string) (os.FileInfo, error) {
		statCalls++
		return dirInfo{}, nil
	})
	r.ctx = ctx

	err := r.Create()

	require.Error(t, err, "a gate that verified nothing must not report success")
	assert.Equal(t, errGateNotCompleted, err)
	assert.Zero(t, statCalls, "the check never ran, which is exactly why this must fail")
	assert.False(t, r.KnownCreated(), "the resource must not be treated as verified")
}

// Cancellation part way through must not blame the daemon for a task the caller
// stopped, because this reason is customer facing.
func TestCreateDoesNotBlameDaemonWhenCancelledMidRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	expectProbe(mockExec, mockCmd, []byte(""), errors.New("Cannot find MPS control daemon process"))

	ctx, cancel := context.WithCancel(context.Background())
	r := newTestResourceWithStat(mockExec, func(string) (os.FileInfo, error) { cancel(); return dirInfo{}, nil })
	r.ctx = ctx

	err := r.Create()

	require.Error(t, err)
	assert.Equal(t, errGateNotCompleted, err)
	assert.NotEqual(t, errDaemonNotResponding.Error(), r.GetTerminalReason(),
		"a task the caller stopped must not be reported as a daemon failure")
}

// A probe that already succeeded means the gate did its job, so later
// cancellation must not turn a verified gate into a failure.
func TestCreateSucceedsWhenCancelledAfterVerification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	expectProbe(mockExec, mockCmd, []byte("100.0\n"), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel inside the attempt, so cancellation is in effect by the time Create
	// evaluates the result of a probe that succeeded.
	r := newTestResourceWithStat(mockExec, func(string) (os.FileInfo, error) { cancel(); return dirInfo{}, nil })
	r.ctx = ctx

	err := r.Create()

	assert.NoError(t, err, "a gate that verified the daemon has done its job")
	assert.Empty(t, r.GetTerminalReason())
}

// The gate timeout and the retry budget interact in a way worth pinning. The
// timeout only decides whether another attempt starts, so when probes burn their
// full timeout the budget is truncated rather than honoured. These assertions fail
// if anyone changes the numbers without revisiting the comments that describe it.
func TestRetryBudgetAgainstGateTimeout(t *testing.T) {
	require.GreaterOrEqual(t, probeMaxAttempts, 1, "a budget of zero attempts would spin")

	// Sum of the delays between attempts, with jitter at its maximum since jitter is
	// only ever added.
	var delays time.Duration
	base := probeMinRetryDelay
	for i := 0; i < probeMaxAttempts-1; i++ {
		delays += time.Duration(float64(base) * (1 + probeRetryJitter))
		if base*time.Duration(probeRetryMultiplier) < probeMaxRetryDelay {
			base *= time.Duration(probeRetryMultiplier)
		} else {
			base = probeMaxRetryDelay
		}
	}

	// The case the schedule is tuned for: an absent daemon fails the probe at once,
	// so only the delays matter and every attempt runs inside the timeout.
	assert.Less(t, delays, defaultGateTimeout,
		"with fast-failing probes every attempt must fit inside the gate timeout")
	assert.Greater(t, delays, time.Second,
		"the budget must outlast a 1s daemon restart, which is the reason the retry exists")

	// The wedged case: every probe burns mps.ProbeTimeout and the budget no longer
	// fits, so the timeout truncates it. This is documented behaviour, not an
	// aspiration, and the assertion is deliberately the way round it is.
	worst := delays + time.Duration(probeMaxAttempts)*mps.ProbeTimeout
	assert.Greater(t, worst, defaultGateTimeout,
		"a wedged daemon exceeds the gate timeout; the comments on defaultGateTimeout say so")

	// How many attempts actually start before the deadline in that case.
	var elapsed time.Duration
	attempts := 0
	base = probeMinRetryDelay
	for attempts < probeMaxAttempts && elapsed < defaultGateTimeout {
		attempts++
		elapsed += mps.ProbeTimeout
		if attempts < probeMaxAttempts {
			elapsed += time.Duration(float64(base) * (1 + probeRetryJitter))
			if base*time.Duration(probeRetryMultiplier) < probeMaxRetryDelay {
				base *= time.Duration(probeRetryMultiplier)
			} else {
				base = probeMaxRetryDelay
			}
		}
	}
	assert.Equal(t, 2, attempts,
		"a wedged daemon gets 2 of the 3 attempts, so \"no success in 2 attempts\" is expected in logs")
}

// The default backoff must produce delays at or above their base, since jitter is
// only ever added.
func TestDefaultBackoff(t *testing.T) {
	b := NewMPSDaemonResource(context.Background(), testTaskARN, nil, "").newBackoff()
	first := b.Duration()
	assert.GreaterOrEqual(t, first, probeMinRetryDelay)
	assert.LessOrEqual(t, first, time.Duration(float64(probeMinRetryDelay)*(1+probeRetryJitter)))
}

// The gate timing out is not the parent being cancelled: the daemon really did
// fail to answer, so the reason must name the daemon rather than an incomplete
// verification.
func TestCreateReportsDaemonWhenGateTimesOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	// Only one attempt is queued; the deadline must stop the retry before a second.
	expectProbe(mockExec, mockCmd, []byte(""), errors.New("Cannot find MPS control daemon process"))

	r := newTestResource(mockExec, nil)
	r.gateTimeout = 5 * time.Millisecond
	// A delay longer than the gate timeout, so the deadline expires during it.
	r.newBackoff = func() retry.Backoff {
		return retry.NewExponentialBackoff(50*time.Millisecond, 50*time.Millisecond, 0, 1)
	}

	err := r.Create()

	require.Error(t, err)
	assert.Equal(t, errDaemonNotResponding, err,
		"a gate that timed out with a live parent must report the daemon, not an incomplete check")
	assert.NotEqual(t, errGateNotCompleted.Error(), r.GetTerminalReason())
}

// Initialize takes the cancellable context from ResourceFields on the restart
// path, where the constructor's context is not available.
func TestInitializeAdoptsResourceFieldsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &MPSDaemonResource{}

	r.Initialize(nil, &taskresource.ResourceFields{Ctx: ctx}, 0, 0)

	assert.Equal(t, ctx, r.ctx, "Initialize must adopt the ResourceFields context")
}

func TestStatusTransitions(t *testing.T) {
	r := NewMPSDaemonResource(context.Background(), testTaskARN, nil, "")

	assert.Equal(t, ResourceName, r.GetName())
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonCreated), r.SteadyState())
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonRemoved), r.TerminalStatus())
	assert.Equal(t, "CREATED", r.StatusString(resourcestatus.ResourceStatus(MPSDaemonCreated)))
	assert.False(t, r.DependOnTaskNetwork())
	assert.False(t, r.RequiresExecutionRoleCredentials())
	assert.Nil(t, r.GetContainerDependencies(resourcestatus.ResourceStatus(MPSDaemonCreated)))
	assert.NoError(t, r.Cleanup(), "the gate owns no host state, so cleanup cannot fail")

	assert.False(t, r.KnownCreated())
	r.SetKnownStatus(resourcestatus.ResourceStatus(MPSDaemonCreated))
	assert.True(t, r.KnownCreated())

	assert.False(t, r.DesiredTerminal())
	r.SetDesiredStatus(resourcestatus.ResourceStatus(MPSDaemonRemoved))
	assert.True(t, r.DesiredTerminal())

	// Only the CREATED transition is defined.
	err := r.ApplyTransition(resourcestatus.ResourceStatus(MPSDaemonRemoved))
	assert.Error(t, err, "an undefined transition must be rejected")
	assert.Contains(t, err.Error(), "impossible")
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	created := time.Now().UTC().Truncate(time.Second)
	r := NewMPSDaemonResource(context.Background(), testTaskARN, nil, mps.ProbeCommand)
	r.SetCreatedAt(created)
	r.SetKnownStatus(resourcestatus.ResourceStatus(MPSDaemonCreated))
	r.SetDesiredStatus(resourcestatus.ResourceStatus(MPSDaemonCreated))

	b, err := r.MarshalJSON()
	require.NoError(t, err)

	restored := &MPSDaemonResource{}
	require.NoError(t, restored.UnmarshalJSON(b))

	assert.Equal(t, testTaskARN, restored.taskARN)
	assert.Empty(t, restored.probeCommand,
		"probeCommand is not serialized; applyDefaults supplies the current default")
	assert.Equal(t, created, restored.GetCreatedAt())
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonCreated), restored.GetKnownStatus(),
		"a verified gate must stay verified across a restart rather than re-probing a running task")
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonCreated), restored.GetDesiredStatus())
}

// Dependencies are not serialized, so a resource restored from disk has none
// until Initialize runs. Without it the resource would be unusable after an
// agent restart.
func TestInitializeRestoresDependenciesAfterUnmarshal(t *testing.T) {
	source := NewMPSDaemonResource(context.Background(), testTaskARN, nil, mps.ProbeCommand)
	b, err := source.MarshalJSON()
	require.NoError(t, err)

	restored := &MPSDaemonResource{}
	require.NoError(t, restored.UnmarshalJSON(b))
	assert.Nil(t, restored.exec, "unmarshal alone must not produce dependencies")
	assert.Nil(t, restored.osWrapper)
	assert.Empty(t, restored.probeCommand)

	restored.Initialize(nil, nil, 0, 0)

	assert.NotNil(t, restored.exec, "Initialize must re-establish the exec dependency")
	assert.NotNil(t, restored.osWrapper)
	assert.NotNil(t, restored.newBackoff)
	assert.Equal(t, mps.ProbeCommand, restored.probeCommand)
	assert.NotNil(t, restored.statusToTransitions[resourcestatus.ResourceStatus(MPSDaemonCreated)],
		"Initialize must rebuild the transition table, which is not serialized")
}

// An empty probe command must fall back to the shared default rather than
// running the control utility with no command.
func TestProbeCommandDefaults(t *testing.T) {
	assert.Equal(t, mps.ProbeCommand, NewMPSDaemonResource(context.Background(), testTaskARN, nil, "").probeCommand)
	assert.Equal(t, "get_server_list",
		NewMPSDaemonResource(context.Background(), testTaskARN, nil, "get_server_list").probeCommand)
}

// Create must not panic on a resource restored from disk that never had
// Initialize called, even though nothing does that today.
func TestCreateOnUninitializedResourceDoesNotPanic(t *testing.T) {
	source := NewMPSDaemonResource(context.Background(), testTaskARN, nil, "")
	b, err := source.MarshalJSON()
	require.NoError(t, err)
	restored := &MPSDaemonResource{}
	require.NoError(t, restored.UnmarshalJSON(b))

	// Inject stubs before calling Create. Without them applyDefaults would install
	// the real os.Stat and exec wrapper, and on a host where the pipe directory
	// exists this test would exec the actual control binary with the production
	// backoff.
	restored.osWrapper = statOS{stat: func(string) (os.FileInfo, error) { return nil, errors.New("no such file") }}
	restored.newBackoff = func() retry.Backoff {
		return retry.NewExponentialBackoff(time.Millisecond, time.Millisecond, 0, 1)
	}

	assert.NotPanics(t, func() { _ = restored.Create() })
}

// A regular file where the pipe directory should be is not a usable daemon socket
// directory.
func TestCreateBlocksWhenPipeDirectoryIsAFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)

	r := newTestResourceWithStat(mockExec, func(string) (os.FileInfo, error) { return fileInfo{}, nil })

	err := r.Create()

	require.Error(t, err, "a non-directory must block the task")
	assert.Equal(t, errDaemonNotAvailable.Error(), err.Error())
}

func TestMarshalJSONOnNilReceiver(t *testing.T) {
	var r *MPSDaemonResource
	_, err := r.MarshalJSON()
	assert.Error(t, err, "marshalling a nil resource must error rather than panic")
}

func TestUnmarshalJSONTolerance(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"empty object", `{}`},
		{"null status", `{"knownStatus":null}`},
		{"only task arn", `{"taskARN":"arn"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &MPSDaemonResource{}
			assert.NoError(t, r.UnmarshalJSON([]byte(tc.in)))
		})
	}

	r := &MPSDaemonResource{}
	assert.Error(t, r.UnmarshalJSON([]byte(`{"knownStatus":`)), "malformed JSON must error")
}

func TestStatusUnmarshalRejectsBadInput(t *testing.T) {
	var s MPSDaemonStatus

	require.NoError(t, s.UnmarshalJSON([]byte("null")))
	assert.Equal(t, MPSDaemonStatusNone, s, "null means the zero state")

	assert.Error(t, s.UnmarshalJSON([]byte("42")), "a non-string status must be rejected")
	assert.Equal(t, MPSDaemonStatusNone, s)

	assert.Error(t, s.UnmarshalJSON([]byte(`"BOGUS"`)), "an unknown status must be rejected")
	assert.Equal(t, MPSDaemonStatusNone, s)

	// A one-character body must not be indexed out of range.
	assert.Error(t, s.UnmarshalJSON([]byte(`"`)))
}

func TestAppliedStatus(t *testing.T) {
	r := NewMPSDaemonResource(context.Background(), testTaskARN, nil, "")
	created := resourcestatus.ResourceStatus(MPSDaemonCreated)

	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonStatusNone), r.GetAppliedStatus())
	assert.True(t, r.SetAppliedStatus(created), "a free resource accepts a transition")
	assert.Equal(t, created, r.GetAppliedStatus())
	assert.False(t, r.SetAppliedStatus(created), "a resource already transitioning refuses another")

	// Reaching the applied status clears it so the next transition can be applied.
	r.SetKnownStatus(created)
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonStatusNone), r.GetAppliedStatus())
}

func TestNextKnownState(t *testing.T) {
	r := NewMPSDaemonResource(context.Background(), testTaskARN, nil, "")
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonCreated), r.NextKnownState())
	r.SetKnownStatus(resourcestatus.ResourceStatus(MPSDaemonCreated))
	assert.Equal(t, resourcestatus.ResourceStatus(MPSDaemonRemoved), r.NextKnownState())
}
