package faults

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
)

func StartFault(port uint16, protocol, trafficType string, taskMetadata state.TaskResponse) error {
	err := startBlackHolePort(port, protocol, trafficType, taskMetadata)
	if err != nil {
		return err
	}
	return nil
}

func startBlackHolePort(port uint16, protocol, trafficType string, taskMetadata state.TaskResponse) error {

	ctx := context.Background()
	ctxWithTimeout, cancel := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	defer cancel()
	netNs := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
	cmdName := []string{"nsenter"}
	chainName := "fault-in"
	netNsArg := "--net=" + netNs

	if trafficType == "egress" {
		chainName = "fault-out"
	}

	parameterList := []string{"iptables", "-N", chainName, "-p", protocol, "--dport", strconv.FormatUint(uint64(port), 10), "-j", "DROP"}
	if netNs != "" {
		parameterList = append([]string{netNsArg}, parameterList...)
		// parameterList = []string{netNsArg, "iptables", "-A", chainName}
	}
	parameterList = append(cmdName, parameterList...)
	cmd := exec.CommandContext(ctxWithTimeout, parameterList[0], parameterList[1:]...)

	stdErr := &bytes.Buffer{}
	stdOut := &bytes.Buffer{}
	cmd.Stderr = stdErr
	cmd.Stdout = stdOut

	err := cmd.Run()

	if err != nil {
		logger.Error("Can't run command", logger.Fields{
			"netns":   netNs,
			"command": strings.Join(parameterList, " "),
			"stdErr":  stdErr.String(),
			"stdOut":  stdOut.String(),
			"err":     err,
		})
		return err
	}

	logger.Debug("Running command", logger.Fields{
		"netns":   netNs,
		"command": strings.Join(parameterList, " "),
		"stdErr":  stdErr.String(),
		"stdOut":  stdOut.String(),
		"err":     err,
	})

	return nil
}
