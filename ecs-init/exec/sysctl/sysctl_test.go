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
	"testing"

	"github.com/golang/mock/gomock"
)

func TestNewIpv4RouteLocalNetFailsWhenExecutableNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", fmt.Errorf("Not found"))

	_, err := NewIpv4RouteLocalNet(mockExec)
	if err == nil {
		t.Error("Expected error when executable's path lookup fails")
	}
}

func TestEnable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil)
	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	mockExec.EXPECT().Command(sysctlExecutable, "-w", "net.ipv4.conf.all.route_localnet=1").Return(mockCmd)
	routeLocalNet, err := NewIpv4RouteLocalNet(mockExec)
	if err != nil {
		t.Fatalf("Error creating Ipv4RouteLocalNet object: %v", err)
	}

	err = routeLocalNet.Enable()
	if err != nil {
		t.Fatal("Error enabling route localnet")
	}
}

func TestEnableReturnsErrorOnCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("its all in vain"))
	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	mockExec.EXPECT().Command(sysctlExecutable, "-w", "net.ipv4.conf.all.route_localnet=1").Return(mockCmd)
	routeLocalNet, err := NewIpv4RouteLocalNet(mockExec)
	if err != nil {
		t.Fatalf("Error creating Ipv4RouteLocalNet object: %v", err)
	}

	err = routeLocalNet.Enable()
	if err == nil {
		t.Fatal("Expected error enabling route localnet")
	}
}

func TestParseDefaultRouteLocalNet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	routeLocalNet, err := NewIpv4RouteLocalNet(mockExec)
	if err != nil {
		t.Fatalf("Error creating Ipv4RouteLocalNet object: %v", err)
	}

	validSysCtlOutput := `net.ipv4.conf.all.route_localnet = 1
`
	out := []byte(validSysCtlOutput)
	parsedVal, err := routeLocalNet.parseDefaultRouteLocalNet(out)
	if err != nil {
		t.Fatalf("Error parsing sysctl output: %v", err)
	}

	if parsedVal != 1 {
		t.Errorf("Incorrect value %d parsed from sample output", parsedVal)
	}
}

func TestParseDefaultRouteLocalNetErrorOnInvalidInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	routeLocalNet, err := NewIpv4RouteLocalNet(mockExec)
	if err != nil {
		t.Fatalf("Error creating Ipv4RouteLocalNet object: %v", err)
	}

	out := []byte("doesnotmatter")
	_, err = routeLocalNet.parseDefaultRouteLocalNet(out)
	if err == nil {
		t.Error("Expected error parsing invalid sysctl output")
	}
}

func TestRestoreDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sysCtlOutput := `net.ipv4.conf.default.route_localnet = 0
	`
	mockCmd := NewMockCmd(ctrl)
	gomock.InOrder(
		mockCmd.EXPECT().Output().Return([]byte(sysCtlOutput), nil),
		mockCmd.EXPECT().Output().Return([]byte{0}, nil),
	)
	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	gomock.InOrder(
		mockExec.EXPECT().Command(sysctlExecutable, "net.ipv4.conf.default.route_localnet").Return(mockCmd),
		mockExec.EXPECT().Command(sysctlExecutable, "-w", "net.ipv4.conf.all.route_localnet=0").Return(mockCmd),
	)
	routeLocalNet, err := NewIpv4RouteLocalNet(mockExec)
	if err != nil {
		t.Fatalf("Error creating Ipv4RouteLocalNet object: %v", err)
	}

	err = routeLocalNet.RestoreDefault()
	if err != nil {
		t.Fatal("Error restoring default route localnet")
	}
}

func TestDisableIpv6RouterAdvertisements(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil)
	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	mockExec.EXPECT().Command(sysctlExecutable, "-e", "-w", "net.ipv6.conf.docker0.accept_ra=0").Return(mockCmd)
	ra, err := NewIpv6RouterAdvertisements(mockExec)
	if err != nil {
		t.Fatalf("Error creating NewIpv6RouterAdvertisements object: %v", err)
	}

	err = ra.Disable()
	if err != nil {
		t.Fatal("Error disabling ipv6 ra")
	}
}

func TestNewIpv6RouterAdvertisementsFailsWhenExecutableNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", fmt.Errorf("Not found"))

	_, err := NewIpv6RouterAdvertisements(mockExec)
	if err == nil {
		t.Error("Expected error when executable's path lookup fails")
	}
}

func TestDisableIpv6RouterAdvertisements_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("nope!"))
	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(sysctlExecutable).Return("", nil)
	mockExec.EXPECT().Command(sysctlExecutable, "-e", "-w", "net.ipv6.conf.docker0.accept_ra=0").Return(mockCmd)
	ra, err := NewIpv6RouterAdvertisements(mockExec)
	if err != nil {
		t.Fatalf("Error creating NewIpv6RouterAdvertisements object: %v", err)
	}

	err = ra.Disable()
	if err == nil {
		t.Fatal("Expected error disabling ipv6 ra")
	}
}
