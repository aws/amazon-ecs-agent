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
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
)

func TestNewNetfilterRouteFailsWhenExecutableNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(iptablesExecutable).Return("", fmt.Errorf("Not found"))

	_, err := NewNetfilterRoute(mockExec)
	if err == nil {
		t.Error("Expected error when executable's path lookup fails")
	}
}

func TestCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	// Mock a successful execution of the iptables command to create the
	// route
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-A", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-A", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Create()
	if err != nil {
		t.Errorf("Error creating route: %v", err)
	}
}

func TestCreateErrorOnPreRoutingCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-A", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Create()
	if err == nil {
		t.Error("Expected error creating route")
	}
}

func TestCreateErrorOnOutputChainCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-A", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-A", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Create()
	if err == nil {
		t.Error("Expected error creating route")
	}
}

func TestRemove(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a successful execution of the iptables command to create the
		// route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a successful execution of the iptables command to create the
		// route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Remove()
	if err != nil {
		t.Errorf("Error removing route: %v", err)
	}
}

func TestRemoveErrorOnPreroutingChainCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Remove()
	if err == nil {
		t.Error("Expected error removing route")
	}
}

func TestRemoveErrorOnOutputChainCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Remove()
	if err == nil {
		t.Error("Expected error removing route")
	}
}

func TestRemoveErrorOnPreroutingChainOutputChainCommandsErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "PREROUTING",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "DNAT",
			"--to-destination", localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
		mockExec.EXPECT().Command(iptablesExecutable,
			"-t", "nat",
			"-D", "OUTPUT",
			"-p", "tcp",
			"-d", credentialsProxyIpAddress,
			"--dport", credentialsProxyPort,
			"-j", "REDIRECT",
			"--to-ports", localhostCredentialsProxyPort).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
	)

	route, err := NewNetfilterRoute(mockExec)
	if err != nil {
		t.Fatalf("Error creating netfilter route object: %v", err)
	}

	err = route.Remove()
	if err == nil {
		t.Error("Expected error removing route")
	}
}

func TestGetNatTableArgs(t *testing.T) {
	if !reflect.DeepEqual(getNatTableArgs(), []string{"-t", "nat"}) {
		t.Error("Incorrect arguments returned for nat table")
	}
}

func TestGetPreroutingChainArgs(t *testing.T) {
	preroutingChainAgrs := []string{
		"PREROUTING",
		"-p", "tcp",
		"-d", "169.254.170.2",
		"--dport", "80",
		"-j", "DNAT",
		"--to-destination", "127.0.0.1:51679",
	}
	if !reflect.DeepEqual(getPreroutingChainArgs(), preroutingChainAgrs) {
		t.Error("Incorrect arguments for modifying prerouting chain")
	}
}

func TestGetOutputChainArgs(t *testing.T) {
	outputChainAgrs := []string{
		"OUTPUT",
		"-p", "tcp",
		"-d", "169.254.170.2",
		"--dport", "80",
		"-j", "REDIRECT",
		"--to-ports", "51679",
	}
	if !reflect.DeepEqual(getOutputChainArgs(), outputChainAgrs) {
		t.Error("Incorrect arguments for modifying output chain")
	}
}

func TestGetActionName(t *testing.T) {
	if getActionName(iptablesAppend) != "append" {
		t.Errorf("Incorrect action name returned for %v", iptablesAppend)
	}
	if getActionName(iptablesDelete) != "delete" {
		t.Errorf("Incorrect action name returned for %v", iptablesDelete)
	}
}
