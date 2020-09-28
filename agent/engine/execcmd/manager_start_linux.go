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
package execcmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
)

// RestartAgentIfStopped restarts the ExecCommandAgent in the container passed as parameter, only for ExecCommandAgent-enabled tasks.
// The status of the ExecCommandAgent in the container is retrieved using a docker exec inspect call, using the dockerExecID
// stored in container.GetExecCommandAgentMetadata().DockerExecID.
//
// If container.GetExecCommandAgentMetadata() is nil, or the ExecCommandAgent is still running, no action is taken.
// To actually restart the ExecCommandAgent, this function invokes this instance's StartAgent method.
func (m *manager) RestartAgentIfStopped(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (RestartStatus, error) {
	if !task.IsExecCommandAgentEnabled() {
		seelog.Warnf("Task engine [%s]: an attempt to restart ExecCommandAgent for a non ExecCommandAgent-enabled task was made; container %s", task.Arn, containerId)
		return NotRestarted, nil
	}
	execMD := container.GetExecCommandAgentMetadata()
	if execMD == nil { // This means m.Start has never been invoked
		return NotRestarted, nil
	}
	res, err := m.inspectExecAgentProcess(ctx, client, execMD)
	if err != nil {
		// We don't want to report InspectContainerExec errors, just that we don't know what the status of the agent is
		return Unknown, nil
	}
	if res.Running { // agent still running, nothing to do
		return NotRestarted, nil
	}
	// Restart if not running
	//TODO: [ecs-exec] retry only for certain exit codes?
	seelog.Warnf("Task engine [%s]: ExecCommandAgent Process stopped (exitCode=%d) for container %s, restarting...", task.Arn, res.ExitCode, containerId)
	container.SetExecCommandAgentMetadata(nil)
	err = m.StartAgent(ctx, client, task, container, containerId)
	if err != nil {
		return NotRestarted, err
	}
	return Restarted, nil
}

func (m *manager) inspectExecAgentProcess(ctx context.Context, client dockerapi.DockerClient, execMD *apicontainer.ExecCommandAgentMetadata) (*types.ContainerExecInspect, error) {
	backoff := retry.NewExponentialBackoff(m.retryMinDelay, m.retryMaxDelay, retryJitterMultiplier, retryDelayMultiplier)
	ctx, cancel := context.WithTimeout(ctx, m.inspectRetryTimeout)
	defer cancel()
	var (
		inspectRes *types.ContainerExecInspect
		inspectErr error
	)
	retry.RetryNWithBackoffCtx(ctx, backoff, maxRetries, func() error {
		inspectRes, inspectErr = client.InspectContainerExec(ctx, execMD.DockerExecID, dockerclient.ContainerExecInspectTimeout)
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

// StartAgent idempotently starts the ExecCommandAgent in the container passed as parameter, only for ExecCommandAgent-enabled tasks.
// In order for the ExecCommandAgent to be successfully started, the following conditions need to be met:
//	1) There shall be at most one ExecCommandAgent process running in the container (with the name defined in execcmd.BinName)
//	2) If an ExecCommandAgent process is already running in the container, its PID shall be equal to container.GetExecCommandAgentMetadata().PID
// If any of the previous conditions is violated, ExecCommandAgent's can't be verified and an error is returned.
// If no error is returned, it can be assumed the ExecCommandAgent is started.
func (m *manager) StartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error {
	if !task.IsExecCommandAgentEnabled() {
		seelog.Warnf("Task engine [%s]: an attempt to start ExecCommandAgent for a non ExecCommandAgent-enabled task was made; container %s", task.Arn, containerId)
		return nil
	}
	backoff := retry.NewExponentialBackoff(m.retryMinDelay, m.retryMaxDelay, retryJitterMultiplier, retryDelayMultiplier)
	ctx, cancel := context.WithTimeout(ctx, m.startRetryTimeout)
	defer cancel()
	var startErr error
	retry.RetryNWithBackoffCtx(ctx, backoff, maxRetries, func() error {
		startErr = m.doStartAgent(ctx, client, task, container, containerId)
		if startErr != nil {
			seelog.Warnf("Task engine [%s]: exec command agent failed to start for container %s: %v. Retrying...", task.Arn, containerId, startErr)
		}
		return startErr
	})

	return startErr
}

func (m *manager) doStartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error {
	execMD := container.GetExecCommandAgentMetadata()
	// Guarantee idempotency if the container already has metadata
	if execMD != nil {
		res, err := m.inspectExecAgentProcess(ctx, client, execMD)
		if err != nil {
			seelog.Warnf("Task engine [%s]: could not verify if the ExecCommandAgent was already running for container %s: %v", task.Arn, containerId, err)
		}
		if res.Running { // agent is already running, nothing to do
			seelog.Warnf("Task engine [%s]: an attempt was made to start the ExecCommandAgent but it was already running (%s)", task.Arn, containerId)
			return nil
		}
	}
	execAgentCmd := filepath.Join(ContainerBinDir, BinName)
	execCfg := types.ExecConfig{
		Detach: true,
		Cmd:    []string{execAgentCmd},
	}
	execRes, err := client.CreateContainerExec(ctx, containerId, execCfg, dockerclient.ContainerExecCreateTimeout)
	if err != nil {
		return StartError{error: err, retryable: true}
	}

	seelog.Debugf("Task engine [%s]: created ExecCommandAgent for container: %s -> docker exec id: %s", task.Arn, containerId, execRes.ID)

	err = client.StartContainerExec(ctx, execRes.ID, dockerclient.ContainerExecStartTimeout)
	if err != nil {
		return StartError{error: err, retryable: true}
	}
	seelog.Debugf("Task engine [%s]: sent ExecCommandAgent start signal for container: %s ->  docker exec id: %s", task.Arn, containerId, execRes.ID)

	inspect, err := client.InspectContainerExec(ctx, execRes.ID, dockerclient.ContainerExecInspectTimeout)
	if err != nil {
		return StartError{error: err, retryable: true}
	}
	seelog.Debugf("Task engine [%s]: inspect ExecCommandAgent for container: %s -> pid: %d, exitCode: %d, running:%v, err:%v",
		task.Arn, containerId, inspect.Pid, inspect.ExitCode, inspect.Running, err)

	if !inspect.Running { //TODO: [ecs-exec] retry only for certain exit codes?
		return StartError{
			error:     fmt.Errorf("ExecCommandAgent could not start for container (exitCode=%d)", inspect.ExitCode),
			retryable: true,
		}
	}
	seelog.Infof("Task engine [%s]: started ExecCommandAgent for container: %s ->  docker exec id: %s", task.Arn, containerId, execRes.ID)
	container.SetExecCommandAgentMetadata(&apicontainer.ExecCommandAgentMetadata{
		PID:          strconv.Itoa(inspect.Pid),
		DockerExecID: execRes.ID,
		CMD:          execAgentCmd,
	})
	return nil
}
