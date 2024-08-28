package faults

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
)

type Blackholeport struct {
	ctx context.Context
}

func (bhp *Blackholeport) CheckFaultStatus(request types.NetworkFaultRequest, taskMetadata state.TaskResponse) (string, error) {
	bhpRequest, ok := request.(types.NetworkBlackholePortRequest)
	if !ok {
		logger.Error("Type assertion failed")
	}

	status, err := checkBlackholePortFault(*bhpRequest.Port, *bhpRequest.Protocol, *bhpRequest.TrafficType, taskMetadata)
	if err != nil {
		return status, err
	}

	return status, nil
}

func checkBlackholePortFault(port uint16, protocol, trafficType string, taskMetadata state.TaskResponse) (string, error) {
	ctx := context.Background()
	ctxWithTimeout, cancel := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	defer cancel()

	cmdName := []string{"nsenter"}
	chainName := "fault-in"
	netNs := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
	netNsArg := "--net=" + netNs

	if trafficType == "egress" {
		chainName = "fault-out"
	}

	cmdArgsList := []string{"iptables", "-nL", chainName}
	if netNs != "" {
		cmdArgsList = append([]string{netNsArg}, cmdArgsList...)
	}
	cmdArgsList = append(cmdName, cmdArgsList...)
	cmd := exec.CommandContext(ctxWithTimeout, cmdArgsList[0], cmdArgsList[1:]...)

	stdErr := &bytes.Buffer{}
	stdOut := &bytes.Buffer{}
	cmd.Stderr = stdErr
	cmd.Stdout = stdOut

	err := cmd.Run()
	if err != nil {
		logger.Error("Can't run command", logger.Fields{
			"netns":    netNs,
			"command":  strings.Join(cmdArgsList, " "),
			"stdErr":   stdErr.String(),
			"stdOut":   stdOut.String(),
			"err":      err,
			"port":     port,
			"protocol": protocol,
		})
		return "stopped", err
	}

	return "running", nil
}
