//go:build unit && linux
// +build unit,linux

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

package doctor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	mock_execwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/mps"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// dirInfo is a stub os.FileInfo that reports itself as a directory.
type dirInfo struct{ os.FileInfo }

func (dirInfo) IsDir() bool { return true }

// dirStat reports the pipe directory as present, or statErr if non-nil.
func dirStat(statErr error) func(string) (os.FileInfo, error) {
	return func(string) (os.FileInfo, error) {
		if statErr != nil {
			return nil, statErr
		}
		return dirInfo{}, nil
	}
}

// expectProbe queues the exec call sequence ProbeControlDaemon makes for one probe. A
// non-nil err makes that probe a failure with exit code 1 (daemon not serving).
func expectProbe(mockExec *mock_execwrapper.MockExec, mockCmd *mock_execwrapper.MockCmd,
	out []byte, err error) {
	mockExec.EXPECT().NewExecContextWithTimeout(gomock.Any(), mps.ProbeTimeout).
		DoAndReturn(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, d)
		})
	mockExec.EXPECT().CommandContext(gomock.Any(), mps.ControlBinary).Return(mockCmd)
	mockCmd.EXPECT().SetEnv(gomock.Any())
	mockCmd.EXPECT().SetIOStreams(gomock.Any(), gomock.Any(), gomock.Any())
	mockCmd.EXPECT().CombinedOutput().Return(out, err)
	if err != nil {
		mockExec.EXPECT().ConvertToExitError(err).Return(&exec.ExitError{}, true)
		mockExec.EXPECT().GetExitCode(gomock.Any()).Return(1)
	}
}

// expectTimeoutProbe queues one probe that hangs past the deadline: the context is
// already expired, so ctx.Err() reports DeadlineExceeded and the result is TimedOut.
func expectTimeoutProbe(mockExec *mock_execwrapper.MockExec, mockCmd *mock_execwrapper.MockCmd) {
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

var probeFailErr = errors.New("Cannot find MPS control daemon process")

func TestMpsGetHealthcheckType(t *testing.T) {
	hc := NewMpsDaemonHealthcheck()
	assert.Equal(t, ecstcs.InstanceHealthCheckTypeAcceleratedCompute, hc.GetHealthcheckType())
}

func TestMpsInitialHealth(t *testing.T) {
	hc := NewMpsDaemonHealthcheck()
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusInitializing, hc.GetHealthcheckStatus())
}

// tickKind is the outcome one health-check tick observes when it probes the daemon.
type tickKind int

const (
	tickServing     tickKind = iota // daemon responds; the probe succeeds
	tickFailure                     // probe runs but the daemon is not serving (exit 1)
	tickTimeout                     // probe hangs past the deadline
	tickPipeMissing                 // pipe directory absent; the exec is skipped
)

// tick pairs a probe outcome with the instance status the check must report after it.
type tick struct {
	kind tickKind
	want ecstcs.InstanceHealthCheckStatus
}

// TestMpsRunCheckSequences drives RunCheck through sequences of probe outcomes and
// asserts the reported status after every tick. Because a serving tick zeroes the
// counter, a status-only assertion still proves the reset semantics: a failure that
// follows a success reports Ok where an unbroken streak of the same length would be
// Impaired.
func TestMpsRunCheckSequences(t *testing.T) {
	const (
		ok       = ecstcs.InstanceHealthCheckStatusOk
		impaired = ecstcs.InstanceHealthCheckStatusImpaired
	)
	cases := []struct {
		name  string
		ticks []tick
	}{
		{
			// Anti-flap: below the threshold a failing probe must not report IMPAIRED,
			// so a short restart that lands on a tick is absorbed.
			name:  "below threshold stays ok",
			ticks: []tick{{tickFailure, ok}, {tickFailure, ok}},
		},
		{
			name:  "third consecutive failure impaired",
			ticks: []tick{{tickFailure, ok}, {tickFailure, ok}, {tickFailure, impaired}},
		},
		{
			// The fourth tick is Ok only because the success zeroed the counter; an
			// unbroken streak of four failures would have been Impaired by tick three.
			name:  "success resets counter",
			ticks: []tick{{tickFailure, ok}, {tickFailure, ok}, {tickServing, ok}, {tickFailure, ok}},
		},
		{
			// A single success mid-streak keeps the count from ever reaching the
			// threshold, so the instance never reports Impaired.
			name: "single success mid streak resets",
			ticks: []tick{{tickFailure, ok}, {tickFailure, ok}, {tickServing, ok},
				{tickFailure, ok}, {tickFailure, ok}},
		},
		{
			// A timeout is a failure like any other, and three in a row cross the threshold.
			name:  "timeout counts as failure",
			ticks: []tick{{tickTimeout, ok}, {tickTimeout, ok}, {tickTimeout, impaired}},
		},
		{
			// A missing pipe directory skips the exec and counts as one failure. Unlike
			// the task gate this is not fail-closed, so three such ticks report Impaired
			// and a later serving probe resets to Ok.
			name: "pipe directory missing counts as failure then recovers",
			ticks: []tick{{tickPipeMissing, ok}, {tickPipeMissing, ok},
				{tickPipeMissing, impaired}, {tickServing, ok}},
		},
		{
			// Recovery: once past the threshold a serving probe returns the instance to Ok.
			name: "recovery returns to ok",
			ticks: []tick{{tickFailure, ok}, {tickFailure, ok}, {tickFailure, impaired},
				{tickServing, ok}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := mock_execwrapper.NewMockExec(ctrl)
			mockCmd := mock_execwrapper.NewMockCmd(ctrl)

			pipePresent := true
			statErr := errors.New("no such file or directory")
			hc := newMpsDaemonHealthcheck(mockExec, func(string) (os.FileInfo, error) {
				if pipePresent {
					return dirInfo{}, nil
				}
				return nil, statErr
			})

			for i, tk := range tc.ticks {
				pipePresent = tk.kind != tickPipeMissing
				switch tk.kind {
				case tickServing:
					expectProbe(mockExec, mockCmd, []byte("100.0\n"), nil)
				case tickFailure:
					expectProbe(mockExec, mockCmd, []byte(""), probeFailErr)
				case tickTimeout:
					expectTimeoutProbe(mockExec, mockCmd)
				case tickPipeMissing:
					// probe short-circuits on the stat error; no exec is expected.
				}
				assert.Equalf(t, tk.want, hc.RunCheck(), "tick %d", i)
			}
		})
	}
}

// The transition back to Ok must be observable to the publisher, which sends only on
// change: GetStatusChangeTime advances on recovery.
func TestMpsRecoveryTransitionAdvancesStatusChangeTime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	for i := 0; i < mpsDaemonImpairedThreshold; i++ {
		expectProbe(mockExec, mockCmd, []byte(""), probeFailErr)
	}
	expectProbe(mockExec, mockCmd, []byte("100.0\n"), nil)

	hc := newMpsDaemonHealthcheck(mockExec, dirStat(nil))

	hc.RunCheck()
	hc.RunCheck()
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusImpaired, hc.RunCheck())
	impairedAt := hc.GetStatusChangeTime()

	// A distinct clock reading, so the recovery transition timestamp is provably newer.
	time.Sleep(time.Millisecond)

	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, hc.RunCheck())
	assert.True(t, hc.GetStatusChangeTime().After(impairedAt),
		"recovery is a status change and must advance GetStatusChangeTime")
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusImpaired, hc.GetLastHealthcheckStatus())
}
