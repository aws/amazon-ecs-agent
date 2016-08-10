// Copyright 2015-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package iptables

import (
	"fmt"

	"github.com/aws/amazon-ecs-init/ecs-init/exec"
	log "github.com/cihub/seelog"
)

// iptablesAction enumerates different actions for the iptables command
type iptablesAction string

const (
	iptablesExecutable            = "iptables"
	credentialsProxyIpAddress     = "169.254.170.2"
	credentialsProxyPort          = "80"
	localhostIpAddress            = "127.0.0.1"
	localhostCredentialsProxyPort = "51679"
	// iptablesAppend enumerates the 'append' action
	iptablesAppend iptablesAction = "-A"
	// iptablesDelete enumerates the 'delete' action
	iptablesDelete iptablesAction = "-D"
)

// NetfilterRoute implements the engine.credentialsProxyRoute interface by
// running the external 'iptables' command
type NetfilterRoute struct {
	cmdExec exec.Exec
}

// getNetfilterChainArgsFunc defines a function pointer type that returns
// a slice of arguments for modifying a netfilter chain
type getNetfilterChainArgsFunc func() []string

// NewNetfilterRoute creates a new NetfilterRoute object
func NewNetfilterRoute(cmdExec exec.Exec) (*NetfilterRoute, error) {
	// Return an error if 'iptables' command cannot be found in the path
	_, err := cmdExec.LookPath(iptablesExecutable)
	if err != nil {
		log.Errorf("Error searching '%s' executable: %v", iptablesExecutable, err)
		return nil, err
	}

	return &NetfilterRoute{
		cmdExec: cmdExec,
	}, nil
}

// Create creates the credentials proxy endpoint route in the netfilter table
func (route *NetfilterRoute) Create() error {
	err := route.modifyNetfilterEntry(iptablesAppend, getPreroutingChainArgs)
	if err != nil {
		return err
	}
	return route.modifyNetfilterEntry(iptablesAppend, getOutputChainArgs)
}

// Remove removes the route for the credentials endpoint from the netfilter
// table
func (route *NetfilterRoute) Remove() error {
	preroutingErr := route.modifyNetfilterEntry(iptablesDelete, getPreroutingChainArgs)
	if preroutingErr != nil {
		// Add more context for error in modifying the prerouting chain
		preroutingErr = fmt.Errorf("Error removing prerouting chain entry: %v", preroutingErr)
	}
	outputErr := route.modifyNetfilterEntry(iptablesDelete, getOutputChainArgs)
	if outputErr != nil {
		if preroutingErr != nil {
			// return a combined error message for prerouting and output chains
			return fmt.Errorf("%v; Error removing output chain entry: %v", preroutingErr, outputErr)
		}
		// Add more context for error in modifying the output chain
		return fmt.Errorf("Error removing output chain entry: %v", outputErr)
	}
	return preroutingErr
}

// modifyNetfilterEntry modifies an entry in the netfilter table based on
// the action and the function pointer to get arguments for modifying the
// chain
func (route *NetfilterRoute) modifyNetfilterEntry(action iptablesAction, getNetfilterChainArgs getNetfilterChainArgsFunc) error {
	args := append(getNatTableArgs(), string(action))
	args = append(args, getNetfilterChainArgs()...)
	cmd := route.cmdExec.Command(iptablesExecutable, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error performing action '%s' for credentials proxy endpoint route: %v; raw output: %s", getActionName(action), err, out)
	}

	return err
}

func getNatTableArgs() []string {
	return []string{"-t", "nat"}
}

func getPreroutingChainArgs() []string {
	return []string{
		"PREROUTING",
		"-p", "tcp",
		"-d", credentialsProxyIpAddress,
		"--dport", credentialsProxyPort,
		"-j", "DNAT",
		"--to-destination", localhostIpAddress + ":" + localhostCredentialsProxyPort,
	}
}

func getOutputChainArgs() []string {
	return []string{
		"OUTPUT",
		"-p", "tcp",
		"-d", credentialsProxyIpAddress,
		"--dport", credentialsProxyPort,
		"-j", "REDIRECT",
		"--to-ports", localhostCredentialsProxyPort,
	}
}

func getActionName(action iptablesAction) string {
	if action == iptablesAppend {
		return "append"
	}

	return "delete"
}
