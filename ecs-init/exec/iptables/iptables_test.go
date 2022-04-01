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
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testErr             = errors.New("test error")
	preroutingRouteArgs = []string{
		"-p", "tcp",
		"-d", credentialsProxyIpAddress,
		"--dport", credentialsProxyPort,
		"-j", "DNAT",
		"--to-destination", localhostIpAddress + ":" + localhostCredentialsProxyPort,
	}
	localhostTrafficFilterInputRouteArgs = []string{
		"--dst", localhostNetwork,
		"!", "--src", localhostNetwork,
		"-m", "conntrack",
		"!", "--ctstate", "RELATED,ESTABLISHED,DNAT",
		"-j", "DROP",
	}
	offhostIntrospectionInterface                 = "ens5"
	blockIntrospectionOffhostAccessInputRouteArgs = []string{
		"-p", "tcp",
		"-i", offhostIntrospectionInterface,
		"--dport", agentIntrospectionServerPort,
		"-j", "DROP",
	}
	blockIntrospectionOffhostAccessInterfaceInputRouteArgs = []string{
		"-p", "tcp",
		"-i", "sn0",
		"--dport", agentIntrospectionServerPort,
		"-j", "DROP",
	}
	outputRouteArgs = []string{
		"-p", "tcp",
		"-d", credentialsProxyIpAddress,
		"--dport", credentialsProxyPort,
		"-j", "REDIRECT",
		"--to-ports", localhostCredentialsProxyPort,
	}

	testIPV4RouteInput = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT                                                       
ens5	00000000	01201FAC	0003	0	0	0	00000000	0	0	0                                                                               
ens5	FEA9FEA9	00000000	0005	0	0	0	FFFFFFFF	0	0	0                                                                               
ens5	00201FAC	00000000	0001	0	0	0	00F0FFFF	0	0	0
`
)

func overrideIPRouteInput(ipv4RouteInput string) func() {
	originalv4 := getDefaultNetworkInterfaceIPv4

	getDefaultNetworkInterfaceIPv4 = func() (string, error) {
		return scanIPv4RoutesForDefaultInterface(strings.NewReader(ipv4RouteInput))
	}

	return func() {
		getDefaultNetworkInterfaceIPv4 = originalv4
		// in real environment we'll only set it once, for testing we unset it after executing relevant test cases
		defaultOffhostIntrospectionInterface = ""
	}
}

func TestNewNetfilterRouteFailsWhenExecutableNotFound(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(iptablesExecutable).Return("", fmt.Errorf("Not found"))

	_, err := NewNetfilterRoute(mockExec)
	assert.Error(t, err, "Expected error when executable's path lookup fails")
}

func TestNewNetfilterRouteWithDefaultOffhostIntrospectionInterfaceFallback(t *testing.T) {
	defer overrideIPRouteInput("")()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockExec(ctrl)
	mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil)

	_, err := NewNetfilterRoute(mockExec)
	assert.NoError(t, err)
	assert.Equal(t, defaultOffhostIntrospectionInterface, fallbackOffhostIntrospectionInterface)
}

func TestCreate(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	testCases := []struct {
		setOffhostInterface bool
		inputRouteArgs      []string
	}{
		{
			setOffhostInterface: false,
			inputRouteArgs:      blockIntrospectionOffhostAccessInputRouteArgs,
		},
		{
			setOffhostInterface: true,
			inputRouteArgs:      blockIntrospectionOffhostAccessInterfaceInputRouteArgs,
		},
	}
	for _, tc := range testCases {
		if tc.setOffhostInterface {
			os.Setenv(offhostIntrospectonAccessInterfaceEnv, "sn0")
			defer os.Unsetenv(offhostIntrospectonAccessInterfaceEnv)
		}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockCmd := NewMockCmd(ctrl)
		// Mock a successful execution of the iptables command to create the
		// route
		mockExec := NewMockExec(ctrl)
		gomock.InOrder(
			mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
			mockExec.EXPECT().Command(iptablesExecutable,
				expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
			mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			mockExec.EXPECT().Command(iptablesExecutable,
				expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
			mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			mockExec.EXPECT().Command(iptablesExecutable,
				expectedArgs("filter", "-I", "INPUT", tc.inputRouteArgs)).Return(mockCmd),
			mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			mockExec.EXPECT().Command(iptablesExecutable,
				expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
			mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		)

		route, err := NewNetfilterRoute(mockExec)
		require.NoError(t, err, "Error creating netfilter route object")

		err = route.Create()
		assert.NoError(t, err, "Error creating route")
	}

}

func TestCreateSkipLocalTrafficFilter(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	os.Setenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER", "true")
	defer os.Unsetenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.NoError(t, err, "Error creating route")
}

func TestCreateAllowOffhostIntrospectionAccess(t *testing.T) {
	os.Setenv(offhostIntrospectionAccessConfigEnv, "true")
	defer os.Unsetenv(offhostIntrospectionAccessConfigEnv)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.NoError(t, err, "Error creating route")
}

func TestCreateErrorOnPreRoutingCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.Error(t, err, "Expected error creating route")
}

func TestCreateErrorOnInputChainCommandError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, testErr),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err)

	err = route.Create()
	assert.Error(t, err)
}

func TestCreateErrorOnOutputChainCommandError(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.Error(t, err, "Expected error creating route")
}

func TestRemove(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		// Mock a successful execution of the iptables command to delete the
		// route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.NoError(t, err, "Error removing route")
}

func TestRemoveSkipLocalTrafficFilter(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	os.Setenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER", "true")
	defer os.Unsetenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.NoError(t, err, "Error removing route")
}

func TestRemoveAllowIntrospectionOffhostAccess(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	os.Setenv(offhostIntrospectionAccessConfigEnv, "true")
	defer os.Unsetenv(offhostIntrospectionAccessConfigEnv)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.NoError(t, err, "Error removing route")
}

func TestRemoveErrorOnPreroutingChainCommandError(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to delete the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.Error(t, err, "Expected error removing route")
}

func TestRemoveErrorOnOutputChainCommandError(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to delete the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.Error(t, err, "Expected error removing route")
}

func TestRemoveErrorOnInputChainCommandsErrors(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, testErr),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.Error(t, err, "Expected error removing route")
}

func TestCombinedError(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	err := combinedError(err1, err2)
	require.NotNil(t, err)
	assert.Equal(t, "err1;err2", err.Error())
}

func TestGetTableArgs(t *testing.T) {
	assert.Equal(t, []string{"-t", "nat"}, getTableArgs("nat"))
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
	assert.Equal(t, preroutingChainAgrs, getPreroutingChainArgs(),
		"Incorrect arguments for modifying prerouting chain")
}

func TestGetLocalhostTrafficFilterInputChainArgs(t *testing.T) {
	assert.Equal(t, []string{
		"INPUT",
		"--dst", "127.0.0.0/8",
		"!", "--src", "127.0.0.0/8",
		"-m", "conntrack",
		"!", "--ctstate", "RELATED,ESTABLISHED,DNAT",
		"-j", "DROP",
	}, getLocalhostTrafficFilterInputChainArgs())
}

func TestGetBlockIntrospectionOffhostAccessInputChainArgs(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()
	defaultOffhostIntrospectionInterface, _ = getOffhostIntrospectionInterface()
	assert.Equal(t, []string{
		"INPUT",
		"-p", "tcp",
		"-i", "ens5",
		"--dport", "51678",
		"-j", "DROP",
	}, getBlockIntrospectionOffhostAccessInputChainArgs())
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
	assert.Equal(t, outputChainAgrs, getOutputChainArgs(),
		"Incorrect arguments for modifying output chain")
}

func TestGetActionName(t *testing.T) {
	assert.Equal(t, "append", getActionName(iptablesAppend))
	assert.Equal(t, "insert", getActionName(iptablesInsert))
	assert.Equal(t, "delete", getActionName(iptablesDelete))
}

func expectedArgs(table, action, chain string, args []string) []string {
	return append([]string{"-t", table, action, chain}, args...)
}

func TestScanIPv4RoutesHappyCase(t *testing.T) {
	iface, err := scanIPv4RoutesForDefaultInterface(strings.NewReader(testIPV4RouteInput))
	assert.NoError(t, err)
	assert.Equal(t, offhostIntrospectionInterface, iface)
}

func TestScanIPv4RoutesNoDefaultRoute(t *testing.T) {
	iface, err := scanIPv4RoutesForDefaultInterface(strings.NewReader(""))
	assert.Error(t, err)
	assert.Equal(t, "", iface)
}

func TestScanIPv4RoutesNoDefaultRouteExceptLoopback(t *testing.T) {
	var testInput = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT                                                       
lo	00000000	01201FAC	0003	0	0	0	00000000	0	0	0
`
	iface, err := scanIPv4RoutesForDefaultInterface(strings.NewReader(testInput))
	assert.Error(t, err)
	assert.Equal(t, "", iface)
}

func TestGetOffhostIntrospectionInterfaceWithEnvOverride(t *testing.T) {
	os.Setenv(offhostIntrospectonAccessInterfaceEnv, "test_iface")
	defer os.Unsetenv(offhostIntrospectonAccessInterfaceEnv)

	iface, err := getOffhostIntrospectionInterface()
	assert.NoError(t, err)
	assert.Equal(t, "test_iface", iface)
}

func TestGetOffhostIntrospectionInterfaceUseDefaultV4(t *testing.T) {
	defer overrideIPRouteInput(testIPV4RouteInput)()

	iface, err := getOffhostIntrospectionInterface()
	assert.NoError(t, err)
	assert.Equal(t, offhostIntrospectionInterface, iface)
}

func TestGetOffhostIntrospectionInterfaceFailure(t *testing.T) {
	defer overrideIPRouteInput("")()

	iface, err := getOffhostIntrospectionInterface()
	assert.Error(t, err)
	assert.Equal(t, "", iface)
}
