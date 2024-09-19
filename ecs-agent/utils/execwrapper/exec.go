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

package execwrapper

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// Exec acts as a wrapper to functions exposed by the exec package.
// Having this interface enables us to create mock objects we can use
// for testing.
type Exec interface {
	CommandContext(ctx context.Context, name string, arg ...string) Cmd
	ConvertToExitError(err error) (*exec.ExitError, bool)
	GetExitCode(exitErr *exec.ExitError) int
}

// execWrapper is a placeholder struct which implements the Exec interface.
type execWrapper struct {
}

func NewExec() Exec {
	return &execWrapper{}
}

// CommandContext essentially acts as a wrapper function for exec.CommandContext function.
func (e *execWrapper) CommandContext(ctx context.Context, name string, arg ...string) Cmd {
	return NewCMDContext(ctx, name, arg...)
}

// ConvertToExitError converts an error object to an exec.ExitError
func (e *execWrapper) ConvertToExitError(err error) (*exec.ExitError, bool) {
	exitErr, eok := err.(*exec.ExitError)
	return exitErr, eok
}

// GetExitCode gets the exit code of an exec.ExitError object
func (e *execWrapper) GetExitCode(exitErr *exec.ExitError) int {
	return exitErr.ExitCode()
}

// Cmd acts as a wrapper to functions exposed by the exec.Cmd object.
// Having this interface enables us to create mock objects we can use
// for testing.
type Cmd interface {
	Run() error
	Start() error
	Wait() error
	KillProcess() error
	AppendExtraFiles(...*os.File)
	Args() []string
	SetIOStreams(io.Reader, io.Writer, io.Writer)
	Output() ([]byte, error)
	CombinedOutput() ([]byte, error)
}

type cmdWrapper struct {
	*exec.Cmd
}

// NewCMDContext returns a new cmdWrapper object which will be used to call standard go os exec calls with a context
func NewCMDContext(ctx context.Context, name string, arg ...string) Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	return &cmdWrapper{Cmd: cmd}
}

// NewCMDContext returns a new cmdWrapper object which will be used to call standard go os exec calls
func NewCMD(name string, arg ...string) Cmd {
	cmd := exec.Command(name, arg...)
	return &cmdWrapper{Cmd: cmd}
}

// Run is a wrapper to existing Run() method of the os/exec go library.
// Run starts the specified command and waits for it to complete.
// Returns nil if the command runs and exits with a zero code.
// Otherwise returns an error
func (c *cmdWrapper) Run() error {
	return c.Cmd.Run()
}

// Start is a wrapper to the existing Start() method of the os/exec go library
// Start starts the specified command but does not wait for it to complete.
func (c *cmdWrapper) Start() error {
	return c.Cmd.Start()
}

// Wait is a wrapper to the existing Wait() method of the os/exec go library
// Wait waits for the command started by a Start() call to exit and waits for any copying to stdin or copying from stdout or stderr to complete.
func (c *cmdWrapper) Wait() error {
	return c.Cmd.Wait()
}

func (c *cmdWrapper) KillProcess() error {
	return c.Cmd.Process.Kill()
}

func (c *cmdWrapper) AppendExtraFiles(ef ...*os.File) {
	c.ExtraFiles = append(c.ExtraFiles, ef...)
}

func (c *cmdWrapper) Args() []string {
	return c.Cmd.Args
}

func (c *cmdWrapper) SetIOStreams(stdin io.Reader, stdout io.Writer, stderr io.Writer) {
	if stdin != nil {
		c.Stdin = stdin
	}
	if stdout != nil {
		c.Stdout = stdout
	}
	if stderr != nil {
		c.Stderr = stderr
	}
}

// Output is a wrapper to the existing Output() method of the os/exec go library.
// Output runs the command and returns its standard output as well as the standard error output.
func (c *cmdWrapper) Output() ([]byte, error) {
	return c.Cmd.Output()
}

// CombinedOutput is a wrapper to the existing CombinedOutput() method of the os/exec go library.
// CombinedOutput runs the command and returns its combined standard output and standard error.
func (c *cmdWrapper) CombinedOutput() ([]byte, error) {
	return c.Cmd.CombinedOutput()
}
