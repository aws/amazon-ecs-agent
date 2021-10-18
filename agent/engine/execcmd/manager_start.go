//go:build linux || windows

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

package execcmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
)

// RestartAgentIfStopped restarts the ExecCommandAgent in the container passed as parameter, only for ExecCommandAgent-enabled containers.
// The status of the ExecCommandAgent in the container is retrieved using a docker exec inspect call, using the dockerExecID
// stored in the AgentMetadata.DockerExecID.
//
// If the ExecCommandAgent is still running (or has never started), no action is taken.
// To actually restart the ExecCommandAgent, this function invokes this instance's StartAgent method.
func (m *manager) RestartAgentIfStopped(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (RestartStatus, error) {
	if !IsExecEnabledContainer(container) {
		seelog.Warnf("Task engine [%s]: an attempt to restart ExecCommandAgent for a non ExecCommandAgent-enabled container was made; container %s", task.Arn, containerId)
		return NotRestarted, nil
	}
	ma, _ := container.GetManagedAgentByName(ExecuteCommandAgentName)
	metadata := MapToAgentMetadata(ma.Metadata)
	if !m.isAgentStarted(ma) {
		return NotRestarted, nil
	}
	res, err := m.inspectExecAgentProcess(ctx, client, metadata)
	if err != nil || res == nil {
		// We don't want to report InspectContainerExec errors, just that we don't know what the status of the agent is
		return Unknown, nil
	}
	if res.Running { // agent still running, nothing to do
		return NotRestarted, nil
	}
	// Restart if not running
	//TODO: [ecs-exec] retry only for certain exit codes?
	seelog.Warnf("Task engine [%s]: ExecCommandAgent Process stopped (exitCode=%d) for container %s, restarting...", task.Arn, res.ExitCode, containerId)
	container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
		ID: ma.ID,
	})
	err = m.StartAgent(ctx, client, task, container, containerId)
	if err != nil {
		return NotRestarted, err
	}
	return Restarted, nil
}

func (m *manager) inspectExecAgentProcess(ctx context.Context, client dockerapi.DockerClient, metadata AgentMetadata) (*types.ContainerExecInspect, error) {
	backoff := retry.NewExponentialBackoff(m.retryMinDelay, m.retryMaxDelay, retryJitterMultiplier, retryDelayMultiplier)
	ctx, cancel := context.WithTimeout(ctx, m.inspectRetryTimeout)
	defer cancel()
	var (
		inspectRes *types.ContainerExecInspect
		inspectErr error
	)
	retry.RetryNWithBackoffCtx(ctx, backoff, maxRetries, func() error {
		inspectRes, inspectErr = client.InspectContainerExec(ctx, metadata.DockerExecID, dockerclient.ContainerExecInspectTimeout)
		if inspectErr != nil {
			retryable := true
			if _, ok := inspectErr.(*dockerapi.DockerTimeoutError); ok {
				retryable = false
			}
			return StartError{
				error:     inspectErr,
				retryable: retryable,
			}
		}
		return nil
	})
	return inspectRes, inspectErr
}

// StartAgent idempotently starts the ExecCommandAgent in the container passed as parameter, only for ExecCommandAgent-enabled containers.
// If no error is returned, it can be assumed the ExecCommandAgent is started.
func (m *manager) StartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error {
	if !IsExecEnabledContainer(container) {
		seelog.Warnf("Task engine [%s]: an attempt to start ExecCommandAgent for a non ExecCommandAgent-enabled container was made; container %s", task.Arn, containerId)
		return nil
	}
	ma, _ := container.GetManagedAgentByName(ExecuteCommandAgentName)
	existingMetadata := MapToAgentMetadata(ma.Metadata)
	if ma.ID == "" {
		return errors.New("container has not been initialized: missing UUID")
	}
	// Guarantee idempotency if the container already has been started
	if m.isAgentStarted(ma) {
		res, err := m.inspectExecAgentProcess(ctx, client, existingMetadata)
		if err != nil {
			seelog.Warnf("Task engine [%s]: could not verify if the ExecCommandAgent was already running for container %s: %v", task.Arn, containerId, err)
		} else if res.Running { // agent is already running, nothing to do
			seelog.Warnf("Task engine [%s]: an attempt was made to start the ExecCommandAgent but it was already running (%s)", task.Arn, containerId)
			return nil
		}
	}

	backoff := retry.NewExponentialBackoff(m.retryMinDelay, m.retryMaxDelay, retryJitterMultiplier, retryDelayMultiplier)
	ctx, cancel := context.WithTimeout(ctx, m.startRetryTimeout)
	defer cancel()
	var startErr error

	var execMD *AgentMetadata
	retry.RetryNWithBackoffCtx(ctx, backoff, maxRetries, func() error {
		execMD, startErr = m.doStartAgent(ctx, client, task, ma, containerId)
		if startErr != nil {
			seelog.Warnf("Task engine [%s]: exec command agent failed to start for container %s: %v. Retrying...", task.Arn, containerId, startErr)
		}
		return startErr
	})
	if startErr != nil {
		container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
			ID:     ma.ID,
			Status: status.ManagedAgentStopped,
			Reason: startErr.Error(),
		})
		return startErr
	}
	container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
		ID:            ma.ID,
		Status:        status.ManagedAgentRunning,
		LastStartedAt: time.Now(),
		Metadata:      execMD.ToMap(),
	})
	return nil
}

func (m *manager) doStartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, ma apicontainer.ManagedAgent, containerId string) (*AgentMetadata, error) {
	execAgentCmdBinDir := getExecAgentCmdBinDir(&ma)
	execAgentCmd := filepath.Join(execAgentCmdBinDir, SSMAgentBinName)
	execCfg := types.ExecConfig{
		User:   execAgentCmdUser,
		Detach: true,
		Cmd:    []string{execAgentCmd},
	}
	newMD := &AgentMetadata{}
	execRes, err := client.CreateContainerExec(ctx, containerId, execCfg, dockerclient.ContainerExecCreateTimeout)
	if err != nil {
		return newMD, StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [create]: %v", err), retryable: true}
	}

	seelog.Debugf("Task engine [%s]: created ExecCommandAgent for container: %s -> docker exec id: %s", task.Arn, containerId, execRes.ID)

	err = client.StartContainerExec(ctx, execRes.ID, types.ExecStartCheck{Detach: true, Tty: false}, dockerclient.ContainerExecStartTimeout)
	if err != nil {
		return newMD, StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [pre-start]: %v", err), retryable: true}
	}
	seelog.Debugf("Task engine [%s]: sent ExecCommandAgent start signal for container: %s ->  docker exec id: %s", task.Arn, containerId, execRes.ID)

	inspect, err := client.InspectContainerExec(ctx, execRes.ID, dockerclient.ContainerExecInspectTimeout)
	if err != nil {
		return newMD, StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [inspect]: %v", err), retryable: true}
	}
	seelog.Debugf("Task engine [%s]: inspect ExecCommandAgent for container: %s -> pid: %d, exitCode: %d, running:%v, err:%v",
		task.Arn, containerId, inspect.Pid, inspect.ExitCode, inspect.Running, err)

	if !inspect.Running { //TODO: [ecs-exec] retry only for certain exit codes?
		return newMD, StartError{
			error:     fmt.Errorf("ExecuteCommandAgent process exited with exit code: %d", inspect.ExitCode),
			retryable: true,
		}
	}
	seelog.Infof("Task engine [%s]: started ExecCommandAgent for container: %s ->  docker exec id: %s", task.Arn, containerId, execRes.ID)
	newMD.PID = strconv.Itoa(inspect.Pid)
	newMD.DockerExecID = execRes.ID
	newMD.CMD = execAgentCmd
	return newMD, nil
}
