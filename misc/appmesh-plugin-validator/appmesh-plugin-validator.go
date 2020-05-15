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

package main

import (
	"fmt"
	"os"

	"github.com/coreos/go-iptables/iptables"
)

const (
	ingressChain               = "APPMESH_INGRESS"
	egressChain                = "APPMESH_EGRESS"
	gid                        = "133"
	appPorts                   = "9080,9090"
	proxyEgressPort            = "15001"
	proxyIngressPort           = "15000"
	egressIgnoredPorts         = "80,81"
	egressIgnoredIPv4Addresses = "192.168.100.0/22,163.107.163.107,169.254.170.2,169.254.169.254"
)

func main() {
	if err := validateIPRules(); err != nil {
		fmt.Errorf("Unable to invoke aws-appmesh cni plugin: %v\n", err)
		os.Exit(1)
	}
	os.Exit(42)
}

// validateIPRules validates IP rules created in the container network namespace.
func validateIPRules() error {
	var iptable *iptables.IPTables
	var err error

	if iptable, err = iptables.NewWithProtocol(iptables.ProtocolIPv4); err != nil {
		return err
	}

	// Print detailed info for debugging.
	str, _ := iptable.ListChains("nat")
	fmt.Printf("List Chains:%v", str)

	strs, _ := iptable.Stats("nat", ingressChain)
	for _, str = range strs {
		fmt.Printf("Ingress chain stats:%v", str)
	}
	strs, _ = iptable.Stats("nat", egressChain)
	for _, str = range strs {
		fmt.Printf("Egress chain stats:%v", str)
	}

	if exist, err := iptable.Exists("nat", ingressChain, "-p", "tcp", "-m", "multiport", "--dports",
		appPorts, "-j", "REDIRECT", "--to-port", proxyIngressPort); !exist {
		return fmt.Errorf("failed to set rules to redirect app ports to proxy: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", "PREROUTING", "-p", "tcp", "-m", "addrtype", "!",
		"--src-type", "LOCAL", "-j", ingressChain); !exist {
		return fmt.Errorf("failed to set rule to jump from PREROUTING to ingress chain: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", egressChain, "-m", "owner", "--gid-owner", gid, "-j", "RETURN"); !exist {
		return fmt.Errorf("failed to set ignoredGID: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", egressChain, "-p", "tcp", "-m", "multiport", "--dports",
		egressIgnoredPorts, "-j", "RETURN"); !exist {
		return fmt.Errorf("failed to set egressIgnoredPorts: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", egressChain, "-p", "tcp", "-d", egressIgnoredIPv4Addresses,
		"-j", "RETURN"); !exist {
		return fmt.Errorf("failed to set egressIgnoredIPv4IPs: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", egressChain, "-p", "tcp", "-j", "REDIRECT", "--to",
		proxyEgressPort); !exist {
		return fmt.Errorf("failed to set rule to redirect traffic to proxyEgressPort: %v\n", err)
	}

	if exist, err := iptable.Exists("nat", "OUTPUT", "-p", "tcp", "-m", "addrtype", "!",
		"--dst-type", "LOCAL", "-j", egressChain); !exist {
		return fmt.Errorf("failed to set rule to jump from OUTPUT to egress chain: %v\n", err)
	}

	return nil
}
