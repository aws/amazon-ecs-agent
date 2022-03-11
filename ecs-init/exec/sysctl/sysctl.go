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

package sysctl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-init/exec"
	log "github.com/cihub/seelog"
)

const (
	sysctlExecutable                  = "sysctl"
	defaultIpv4RouteLocalnetConfigKey = "net.ipv4.conf.default.route_localnet"
	allIpv4RouteLocalnetConfigKey     = "net.ipv4.conf.all.route_localnet"
	dockerIpv6AcceptRAKey             = "net.ipv6.conf.docker0.accept_ra"
)

// Ipv4RouteLocalnet implements the engine.loopbackRouting interface by
// running the external 'sysctl' command
type Ipv4RouteLocalnet struct {
	cmdExec exec.Exec
}

// NewIpv4RouteLocalNet creates a new Ipv4RouteLocalnet object
func NewIpv4RouteLocalNet(cmdExec exec.Exec) (*Ipv4RouteLocalnet, error) {
	_, err := cmdExec.LookPath(sysctlExecutable)
	if err != nil {
		log.Errorf("Error searching '%s' executable: %v", sysctlExecutable, err)
		return nil, err
	}

	return &Ipv4RouteLocalnet{
		cmdExec: cmdExec,
	}, nil
}

// Ipv6RouterAdvertisements implements the engine.ipv6RouterAdvertisements interface
// by running the external 'sysctl' command
type Ipv6RouterAdvertisements struct {
	cmdExec exec.Exec
}

// NewIpv6RouterAdvertisements creates a new Ipv6RouterAdvertisements object
func NewIpv6RouterAdvertisements(cmdExec exec.Exec) (*Ipv6RouterAdvertisements, error) {
	_, err := cmdExec.LookPath(sysctlExecutable)
	if err != nil {
		log.Errorf("Error searching '%s' executable: %v", sysctlExecutable, err)
		return nil, err
	}

	return &Ipv6RouterAdvertisements{
		cmdExec: cmdExec,
	}, nil
}

// Disable disables ipv6 router advertisements
func (ra *Ipv6RouterAdvertisements) Disable() error {
	cmd := ra.cmdExec.Command(sysctlExecutable, "-e", "-w", fmt.Sprintf("%s=0", dockerIpv6AcceptRAKey))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error disable ipv6 router advertisements %v; raw output: %s", err, out)
	}
	return err
}

// Enable enables routing to loopback addresses
func (ipv4RouteLocalnet *Ipv4RouteLocalnet) Enable() error {
	cmd := ipv4RouteLocalnet.cmdExec.Command(sysctlExecutable, "-w", fmt.Sprintf("%s=1", allIpv4RouteLocalnetConfigKey))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error enabling route_localnet %v; raw output: %s", err, out)
	}
	return err
}

// Restore restores the default value for loopback addresses
func (ipv4RouteLocalnet *Ipv4RouteLocalnet) RestoreDefault() error {
	cmd := ipv4RouteLocalnet.cmdExec.Command(sysctlExecutable, defaultIpv4RouteLocalnetConfigKey)
	out, err := cmd.Output()
	if err != nil {
		log.Errorf("Error getting default route_localnet %v; raw output: %s", err, out)
		return err
	}
	defaultVal, err := ipv4RouteLocalnet.parseDefaultRouteLocalNet(out)
	if err != nil {
		return err
	}

	cmd = ipv4RouteLocalnet.cmdExec.Command(sysctlExecutable, "-w", fmt.Sprintf("%s=%d", allIpv4RouteLocalnetConfigKey, defaultVal))
	out, err = cmd.Output()
	if err != nil {
		log.Errorf("Error restoring all route_localnet %v; raw output: %s", err, out)
	}
	return err
}

// parse parses the default route_localnet value from the output of
// 'sysctl net.ipv4.conf.default.route_localnet' command
func (Ipv4RouteLocalnet *Ipv4RouteLocalnet) parseDefaultRouteLocalNet(out []byte) (int64, error) {
	s := string(out)
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		log.Errorf("Unexpected value read when determining default value for route_localnet, %d parts parsed for %s", len(parts), s)
		return 0, fmt.Errorf("Errpr parsing default route_localnet value")
	}

	val, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}
