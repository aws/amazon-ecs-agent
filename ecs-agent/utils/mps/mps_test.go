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

package mps

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"

	mock_execwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func uintPtr(v uint) *uint { return &v }

func TestPinnedDeviceMemLimit(t *testing.T) {
	// Always keyed on in-container device index 0, MiB value, "M" suffix.
	assert.Equal(t, "0=1024M", PinnedDeviceMemLimit(1024))
	assert.Equal(t, "0=1M", PinnedDeviceMemLimit(1))
	assert.Equal(t, "0=22563M", PinnedDeviceMemLimit(22563))
}

func TestBuildEnvWithComputePercent(t *testing.T) {
	env := BuildEnv(4096, uintPtr(50))
	assert.Equal(t, map[string]string{
		PipeDirectoryEnvVar:          PipeDirectory,
		PinnedDeviceMemLimitEnvVar:   "0=4096M",
		ActiveThreadPercentageEnvVar: "50",
	}, env)
}

func TestBuildEnvWithoutComputePercent(t *testing.T) {
	// When compute percent is omitted, the active-thread env var must be absent
	// entirely (daemon default of 100% applies), not set to 0 or 100.
	env := BuildEnv(4096, nil)
	assert.Equal(t, map[string]string{
		PipeDirectoryEnvVar:        PipeDirectory,
		PinnedDeviceMemLimitEnvVar: "0=4096M",
	}, env)
	_, ok := env[ActiveThreadPercentageEnvVar]
	assert.False(t, ok, "compute percent env var must be omitted when nil")
}

func TestBuildEnvComputePercentBoundaries(t *testing.T) {
	env := BuildEnv(1, uintPtr(1))
	assert.Equal(t, "1", env[ActiveThreadPercentageEnvVar])

	env = BuildEnv(1, uintPtr(100))
	assert.Equal(t, "100", env[ActiveThreadPercentageEnvVar])
}

// newProbeMocks wires a MockExec+MockCmd so ProbeControlDaemon can run against a
// canned CombinedOutput result. It captures the stdin reader passed to
// SetIOStreams into gotStdin so a test can assert the command is fed on stdin.
func newProbeMocks(t *testing.T, gotStdin *string, gotEnv *[]string) (*mock_execwrapper.MockExec, *mock_execwrapper.MockCmd, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	mockExec.EXPECT().NewExecContextWithTimeout(gomock.Any(), ProbeTimeout).
		DoAndReturn(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, d)
		})
	mockExec.EXPECT().CommandContext(gomock.Any(), ControlBinary).Return(mockCmd)
	mockCmd.EXPECT().SetEnv(gomock.Any()).
		Do(func(env []string) {
			if gotEnv != nil {
				*gotEnv = env
			}
		})
	mockCmd.EXPECT().SetIOStreams(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(stdin io.Reader, stdout, stderr io.Writer) {
			if gotStdin != nil && stdin != nil {
				b, _ := io.ReadAll(stdin)
				*gotStdin = string(b)
			}
		})
	return mockExec, mockCmd, ctrl
}

func TestProbeControlDaemonServing(t *testing.T) {
	var gotStdin string
	var gotEnv []string
	mockExec, mockCmd, ctrl := newProbeMocks(t, &gotStdin, &gotEnv)
	defer ctrl.Finish()
	// Control utility exits 0 and prints the default active-thread percentage.
	mockCmd.EXPECT().CombinedOutput().Return([]byte("100.0\n"), nil)

	res := ProbeControlDaemon(mockExec, ProbeCommand)
	assert.NoError(t, res.Err, "a serving daemon must produce no error")
	assert.Equal(t, 0, res.ExitCode)
	assert.False(t, res.TimedOut)
	assert.Equal(t, "100.0", res.Stdout)
	assert.Equal(t, ProbeCommand+"\n", gotStdin, "the probe command must be fed on stdin")
	assert.Contains(t, gotEnv, PipeDirectoryEnvVar+"="+PipeDirectory,
		"the probe must tell the control utility where the daemon listens")
}

func TestProbeControlDaemonNotServing(t *testing.T) {
	mockExec, mockCmd, ctrl := newProbeMocks(t, nil, nil)
	defer ctrl.Finish()
	// Control utility exits nonzero: daemon not found or connection broken.
	exitErr := &exec.ExitError{}
	mockCmd.EXPECT().CombinedOutput().Return([]byte("connection failed"), exitErr)
	mockExec.EXPECT().ConvertToExitError(exitErr).Return(exitErr, true)
	mockExec.EXPECT().GetExitCode(exitErr).Return(1)

	res := ProbeControlDaemon(mockExec, ProbeCommand)
	assert.Error(t, res.Err, "a non-serving daemon must produce an error")
	assert.Equal(t, 1, res.ExitCode)
	assert.False(t, res.TimedOut)
}

func TestProbeControlDaemonWedged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := mock_execwrapper.NewMockExec(ctrl)
	mockCmd := mock_execwrapper.NewMockCmd(ctrl)
	// A zero-duration timeout makes the context deadline fire immediately.
	mockExec.EXPECT().NewExecContextWithTimeout(gomock.Any(), ProbeTimeout).
		DoAndReturn(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 0)
		})
	mockExec.EXPECT().CommandContext(gomock.Any(), ControlBinary).Return(mockCmd)
	mockCmd.EXPECT().SetEnv(gomock.Any())
	mockCmd.EXPECT().SetIOStreams(gomock.Any(), gomock.Any(), gomock.Any())
	killErr := errors.New("signal: killed")
	mockCmd.EXPECT().CombinedOutput().Return([]byte{}, killErr)
	mockExec.EXPECT().ConvertToExitError(killErr).Return(nil, false)

	res := ProbeControlDaemon(mockExec, ProbeCommand)
	assert.Error(t, res.Err, "a wedged daemon must produce an error")
	assert.True(t, res.TimedOut, "a deadline-killed probe must report TimedOut")
	assert.Equal(t, -1, res.ExitCode, "a non-ExitError failure must report exit code -1")
}
