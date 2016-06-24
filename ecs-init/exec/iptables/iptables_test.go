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
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable, "-t", "nat", "-A", "PREROUTING",
			"-p", "tcp", "-d", credentialsProxyIpAddress, "--dport",
			credentialsProxyPort, "-j", "DNAT", "--to-destination",
			localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
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

func TestCreateErrorOnCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	// Mock a failed execution of the iptables command to create the route
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?"))
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable, "-t", "nat", "-A", "PREROUTING",
			"-p", "tcp", "-d", credentialsProxyIpAddress, "--dport",
			credentialsProxyPort, "-j", "DNAT", "--to-destination",
			localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
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
	// Mock a successful execution of the iptables command to create the
	// route
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable, "-t", "nat", "-D", "PREROUTING",
			"-p", "tcp", "-d", credentialsProxyIpAddress, "--dport",
			credentialsProxyPort, "-j", "DNAT", "--to-destination",
			localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
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

func TestRemoveErrorOnCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	// Mock a failed execution of the iptables command to create the route
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry"))
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable, "-t", "nat", "-D", "PREROUTING",
			"-p", "tcp", "-d", credentialsProxyIpAddress, "--dport",
			credentialsProxyPort, "-j", "DNAT", "--to-destination",
			localhostIpAddress+":"+localhostCredentialsProxyPort).Return(mockCmd),
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
