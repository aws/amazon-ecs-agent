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
	"os"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-init/ecs-init/exec"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// iptablesAction enumerates different actions for the iptables command
type iptablesAction string

const (
	iptablesExecutable            = "iptables"
	credentialsProxyIpAddress     = "169.254.170.2"
	credentialsProxyPort          = "80"
	localhostIpAddress            = "127.0.0.1"
	localhostCredentialsProxyPort = "51679"
	localhostNetwork              = "127.0.0.0/8"
	// iptablesAppend enumerates the 'append' action
	iptablesAppend iptablesAction = "-A"
	// iptablesInsert enumerates the 'insert' action
	iptablesInsert iptablesAction = "-I"
	// iptablesDelete enumerates the 'delete' action
	iptablesDelete iptablesAction = "-D"

	iptablesTableFilter = "filter"
	iptablesTableNat    = "nat"

	offhostIntrospectionAccessConfigEnv   = "ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS"
	offhostIntrospectonAccessInterfaceEnv = "ECS_OFFHOST_INTROSPECTION_INTERFACE_NAME"
	agentIntrospectionServerPort          = "51678"
	defaultOffhostIntrospectionInterface  = "eth0"
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
	err := route.modifyNetfilterEntry(iptablesTableNat, iptablesAppend, getPreroutingChainArgs)
	if err != nil {
		return err
	}

	if !skipLocalhostTrafficFilter() {
		err = route.modifyNetfilterEntry(iptablesTableFilter, iptablesInsert, getLocalhostTrafficFilterInputChainArgs)
		if err != nil {
			return err
		}
	}

	if !allowOffhostIntrospection() {
		err = route.modifyNetfilterEntry(iptablesTableFilter, iptablesInsert, getBlockIntrospectionOffhostAccessInputChainArgs)
		if err != nil {
			log.Errorf("Error adding input chain entry to block offhost introspection access: %v", err)
		}
	}

	return route.modifyNetfilterEntry(iptablesTableNat, iptablesAppend, getOutputChainArgs)
}

// Remove removes the route for the credentials endpoint from the netfilter
// table
func (route *NetfilterRoute) Remove() error {
	preroutingErr := route.modifyNetfilterEntry(iptablesTableNat, iptablesDelete, getPreroutingChainArgs)
	if preroutingErr != nil {
		// Add more context for error in modifying the prerouting chain
		preroutingErr = fmt.Errorf("error removing prerouting chain entry: %v", preroutingErr)
	}

	var localhostInputError, introspectionInputError error
	if !skipLocalhostTrafficFilter() {
		localhostInputError = route.modifyNetfilterEntry(iptablesTableFilter, iptablesDelete, getLocalhostTrafficFilterInputChainArgs)
		if localhostInputError != nil {
			localhostInputError = fmt.Errorf("error removing input chain entry: %v", localhostInputError)
		}
	}

	introspectionInputError = route.modifyNetfilterEntry(iptablesTableFilter, iptablesDelete, getBlockIntrospectionOffhostAccessInputChainArgs)
	if introspectionInputError != nil {
		introspectionInputError = fmt.Errorf("error removing input chain entry: %v", introspectionInputError)
	}

	outputErr := route.modifyNetfilterEntry(iptablesTableNat, iptablesDelete, getOutputChainArgs)
	if outputErr != nil {
		// Add more context for error in modifying the output chain
		outputErr = fmt.Errorf("error removing output chain entry: %v", outputErr)
	}

	return combinedError(preroutingErr, localhostInputError, introspectionInputError, outputErr)
}

func combinedError(errs ...error) error {
	errMsgs := []string{}
	for _, err := range errs {
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		}
	}

	if len(errMsgs) == 0 {
		return nil
	}

	return errors.New(strings.Join(errMsgs, ";"))
}

// modifyNetfilterEntry modifies an entry in the netfilter table based on
// the action and the function pointer to get arguments for modifying the
// chain
func (route *NetfilterRoute) modifyNetfilterEntry(table string, action iptablesAction, getNetfilterChainArgs getNetfilterChainArgsFunc) error {
	args := append(getTableArgs(table), string(action))
	args = append(args, getNetfilterChainArgs()...)
	cmd := route.cmdExec.Command(iptablesExecutable, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error performing action '%s' for iptables route: %v; raw output: %s", getActionName(action), err, out)
	}

	return err
}

func getTableArgs(table string) []string {
	return []string{"-t", table}
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

func getLocalhostTrafficFilterInputChainArgs() []string {
	return []string{
		"INPUT",
		"--dst", localhostNetwork,
		"!", "--src", localhostNetwork,
		"-m", "conntrack",
		"!", "--ctstate", "RELATED,ESTABLISHED,DNAT",
		"-j", "DROP",
	}
}

func getBlockIntrospectionOffhostAccessInputChainArgs() []string {
	return []string{
		"INPUT",
		"-p", "tcp",
		"-i", getOffhostIntrospectionInterface(),
		"--dport", agentIntrospectionServerPort,
		"-j", "DROP",
	}
}

func getOffhostIntrospectionInterface() string {
	s := os.Getenv(offhostIntrospectonAccessInterfaceEnv)
	if s != "" {
		return s
	}
	return defaultOffhostIntrospectionInterface
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
	switch action {
	case iptablesAppend:
		return "append"
	case iptablesInsert:
		return "insert"
	default:
		return "delete"
	}
}

func skipLocalhostTrafficFilter() bool {
	s := os.Getenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER")
	if s == "" {
		return false
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		log.Errorf("Failed to parse value for ECS_SKIP_LOCALHOST_TRAFFIC_FILTER [%s]: %v. Default it to false.", s, err)
		return false
	}
	return b
}

func allowOffhostIntrospection() bool {
	s := os.Getenv(offhostIntrospectionAccessConfigEnv)
	if s == "" {
		return false
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		log.Errorf("Failed to parse value for %s [%s]: %v. Default it to false.",
			offhostIntrospectionAccessConfigEnv, s, err)
		return false
	}
	return b
}
