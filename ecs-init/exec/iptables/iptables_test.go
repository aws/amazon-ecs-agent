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
	"net"
	"os"
	"testing"

	mock_nlwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

const (
	testLoopbackInterfaceName     = "lo0"
	testDockerBridgeInterfaceName = "docker0"
)

var (
	testErr             = errors.New("test error")
	preroutingRouteArgs = []string{
		"-p", "tcp",
		"-d", CredentialsProxyIpAddress,
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

	allowIntrospectionForDockerArgs = []string{
		"-p", "tcp",
		"--dport", agentIntrospectionServerPort,
		"-i", testDockerBridgeInterfaceName,
		"-j", "ACCEPT",
	}
	blockIntrospectionOffhostAccessInputRouteArgs = []string{
		"-p", "tcp",
		"--dport", agentIntrospectionServerPort,
		"!", "-i", testLoopbackInterfaceName,
		"-j", "DROP",
	}

	defaultLoLink = &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Index: 1,
			Flags: net.FlagLoopback,
			Name:  testLoopbackInterfaceName,
		},
	}
	outputRouteArgs = []string{
		"-p", "tcp",
		"-d", CredentialsProxyIpAddress,
		"--dport", credentialsProxyPort,
		"-j", "REDIRECT",
		"--to-ports", localhostCredentialsProxyPort,
	}
	osStatFileNotFoundError = fmt.Errorf("file not found")
)

func resetDefaultNetworkInterfaceVariables() func() {
	return func() {
		defaultLoopbackInterfaceName = ""
		defaultDockerBridgeNetworkName = ""
	}
}

func TestNewNetfilterRoute(t *testing.T) {

	testCases := []struct {
		name                          string
		shouldError                   bool
		setMockExpectations           func(mockExec *MockExec, mockNetlink *mock_nlwrapper.MockNetLink)
		expectedLoopbackInterfaceName string
	}{
		{
			name:        "success",
			shouldError: false,
			setMockExpectations: func(mockExec *MockExec, mockNetlink *mock_nlwrapper.MockNetLink) {
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil)
				mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
			},
			expectedLoopbackInterfaceName: testLoopbackInterfaceName,
		},
		{
			name:        "executable not found",
			shouldError: true,
			setMockExpectations: func(mockExec *MockExec, mockNetlink *mock_nlwrapper.MockNetLink) {
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", fmt.Errorf("Not found"))
			},
		},
		{
			name:        "default loopback not found",
			shouldError: true,
			setMockExpectations: func(mockExec *MockExec, mockNetlink *mock_nlwrapper.MockNetLink) {
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil)
				mockNetlink.EXPECT().LinkList().Return(nil, fmt.Errorf("loopback not found"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer resetDefaultNetworkInterfaceVariables()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockExec(ctrl)
			mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
			if tc.setMockExpectations != nil {
				tc.setMockExpectations(mockExec, mockNetlink)
			}

			netRoute, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, netRoute)
				assert.Equal(t, tc.expectedLoopbackInterfaceName, defaultLoopbackInterfaceName)
			}
		})
	}

}

func appendIpv6Mocks(mockExec *MockExec, mockCmd *MockCmd, mocks []*gomock.Call, table, action, chain string, args []string, useIp6tables bool, osStatError error) []*gomock.Call {
	mocks = append(mocks,
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs(table, action, chain, args)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)
	if osStatError == nil {
		if useIp6tables {
			mocks = append(mocks,
				mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
				mockExec.EXPECT().Command(ip6tablesExecutable,
					expectedArgs(table, action, chain, args)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			)
		} else {
			mocks = append(mocks,
				mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", errors.New("error")),
			)
		}
	}

	return mocks
}

func TestCreate(t *testing.T) {

	testCases := []struct {
		name         string
		useIp6tables bool
		osStatError  error
	}{
		{
			name:         "create iptables route",
			useIp6tables: false,
			osStatError:  nil,
		},
		{
			name:         "create ip6tables route",
			useIp6tables: true,
			osStatError:  nil,
		},
		{
			name:         "create ip6tables route when ipv6 kernel configs not found",
			useIp6tables: true,
			osStatError:  osStatFileNotFoundError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer resetDefaultNetworkInterfaceVariables()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			defer func() {
				checkForPath = os.Stat
			}()
			checkForPath = func(name string) (os.FileInfo, error) {
				return nil, tc.osStatError
			}

			mockCmd := NewMockCmd(ctrl)
			// Mock a successful execution of the iptables command to create the
			// route
			mockExec := NewMockExec(ctrl)
			mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
			mocks := []*gomock.Call{
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
				mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),

				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),

				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			}

			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesInsert), "INPUT", blockIntrospectionOffhostAccessInputRouteArgs, tc.useIp6tables, tc.osStatError)
			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesInsert), "INPUT", allowIntrospectionForDockerArgs, tc.useIp6tables, tc.osStatError)

			mocks = append(mocks,
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			)

			gomock.InOrder(
				mocks...,
			)

			route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
			require.NoError(t, err, "Error creating netfilter route object")

			err = route.Create()
			assert.NoError(t, err, "Error creating route")
		})
	}
}

func TestCreateSkipLocalTrafficFilter(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	os.Setenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER", "true")
	defer os.Unsetenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()
	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-I", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),

		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.NoError(t, err, "Error creating route")
}

func TestCreateAllowOffhostIntrospectionAccess(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	os.Setenv(offhostIntrospectionAccessConfigEnv, "true")
	defer os.Unsetenv(offhostIntrospectionAccessConfigEnv)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
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

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.NoError(t, err, "Error creating route")
}

func TestCreateBlockOffhostIntrospectionAccessErrors(t *testing.T) {
	testCases := []struct {
		name             string
		mockExpectations func(mockExec *MockExec, mockCmd *MockCmd, mockNetlink *mock_nlwrapper.MockNetLink)
	}{
		{
			name: "iptables drop connections fail",
			mockExpectations: func(mockExec *MockExec, mockCmd *MockCmd, mockNetlink *mock_nlwrapper.MockNetLink) {
				gomock.InOrder(
					mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
					mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("error unable to drop connections for offhost access to introspection server")),
				)
			},
		},
		{
			name: "iptables allow connections from docker bridge fail",
			mockExpectations: func(mockExec *MockExec, mockCmd *MockCmd, mockNetlink *mock_nlwrapper.MockNetLink) {
				gomock.InOrder(
					mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
					mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
					mockExec.EXPECT().Command(ip6tablesExecutable,
						expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
					mockExec.EXPECT().Command(iptablesExecutable,
						expectedArgs("filter", "-I", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
					mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("error unable to accept connections from docker bridge interface to introspection server")),
				)
			},
		},
	}

	defer func() {
		checkForPath = os.Stat
	}()
	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer resetDefaultNetworkInterfaceVariables()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockExec(ctrl)
			mockCmd := NewMockCmd(ctrl)
			mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)

			tc.mockExpectations(mockExec, mockCmd, mockNetlink)

			route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
			require.NoError(t, err, "Error creating netfilter route object")

			err = route.Create()
			assert.Error(t, err)
		})
	}
}

func TestCreateErrorOnPreRoutingCommandError(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.Error(t, err, "Expected error creating route")
}

func TestCreateErrorOnInputChainCommandError(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, testErr),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err)

	err = route.Create()
	assert.Error(t, err)
}

func TestCreateErrorOnOutputChainCommandError(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()
	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-I", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-I", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-I", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-A", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to create the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("didn't expect this, did you?")),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Create()
	assert.Error(t, err, "Expected error creating route")
}

func TestRemove(t *testing.T) {
	testCases := []struct {
		name         string
		useIp6tables bool
		osStatError  error
	}{
		{
			name:         "test remove with iptables only",
			useIp6tables: false,
			osStatError:  nil,
		},
		{
			name:         "test remove with ip6tables",
			useIp6tables: true,
			osStatError:  nil,
		},
		{
			name:         "test remove with ip6tables with file not found error",
			useIp6tables: true,
			osStatError:  osStatFileNotFoundError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer resetDefaultNetworkInterfaceVariables()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			defer func() {
				checkForPath = os.Stat
			}()
			checkForPath = func(name string) (os.FileInfo, error) {
				return nil, tc.osStatError
			}

			mockCmd := NewMockCmd(ctrl)
			mockExec := NewMockExec(ctrl)
			mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)

			mocks := []*gomock.Call{
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
				mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),

				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
				// Mock a successful execution of the iptables command to delete the
				// route
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			}

			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesDelete), "INPUT", blockIntrospectionOffhostAccessInputRouteArgs, tc.useIp6tables, tc.osStatError)
			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesDelete), "INPUT", allowIntrospectionForDockerArgs, tc.useIp6tables, tc.osStatError)

			mocks = append(mocks,
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			)

			gomock.InOrder(
				mocks...,
			)

			route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
			require.NoError(t, err, "Error creating netfilter route object")

			err = route.Remove()
			assert.NoError(t, err, "Error removing route")
		})
	}
}

func TestRemoveSkipLocalTrafficFilter(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	os.Setenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER", "true")
	defer os.Unsetenv("ECS_SKIP_LOCALHOST_TRAFFIC_FILTER")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()

	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.NoError(t, err, "Error removing route")
}

func TestRemoveAllowIntrospectionOffhostAccess(t *testing.T) {

	os.Setenv(offhostIntrospectionAccessConfigEnv, "true")
	defer os.Unsetenv(offhostIntrospectionAccessConfigEnv)

	testCases := []struct {
		name         string
		useIp6tables bool
		osStatError  error
	}{
		{
			name:         "remove allow instrocpection off-host access with iptables only",
			useIp6tables: false,
			osStatError:  nil,
		},
		{
			name:         "remove allow instrocpection off-host access with ip6tables",
			useIp6tables: true,
			osStatError:  nil,
		},
		{
			name:         "remove allow instrocpection off-host access with ip6tables and file not found error",
			useIp6tables: true,
			osStatError:  osStatFileNotFoundError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer resetDefaultNetworkInterfaceVariables()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			defer func() {
				checkForPath = os.Stat
			}()
			checkForPath = func(name string) (os.FileInfo, error) {
				return nil, tc.osStatError
			}

			mockCmd := NewMockCmd(ctrl)
			mockExec := NewMockExec(ctrl)
			mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)

			mocks := []*gomock.Call{
				mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
				mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			}
			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesDelete), "INPUT", blockIntrospectionOffhostAccessInputRouteArgs, tc.useIp6tables, tc.osStatError)
			mocks = appendIpv6Mocks(mockExec, mockCmd, mocks, "filter", string(iptablesDelete), "INPUT", allowIntrospectionForDockerArgs, tc.useIp6tables, tc.osStatError)
			mocks = append(mocks,
				mockExec.EXPECT().Command(iptablesExecutable,
					expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
				mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
			)

			gomock.InOrder(
				mocks...,
			)

			route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
			require.NoError(t, err, "Error creating netfilter route object")

			err = route.Remove()
			assert.NoError(t, err, "Error removing route")
		})
	}
}

func TestRemoveErrorOnPreroutingChainCommandError(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()

	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
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
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.Error(t, err, "Expected error removing route")
}

func TestRemoveErrorOnOutputChainCommandError(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()

	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		// Mock a failed execution of the iptables command to delete the route
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, fmt.Errorf("no cpu cycles to spare, sorry")),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
	require.NoError(t, err, "Error creating netfilter route object")

	err = route.Remove()
	assert.Error(t, err, "Expected error removing route")
}

func TestRemoveErrorOnInputChainCommandsErrors(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	defer func() {
		checkForPath = os.Stat
	}()

	checkForPath = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	mockCmd := NewMockCmd(ctrl)
	mockExec := NewMockExec(ctrl)
	mockNetlink := mock_nlwrapper.NewMockNetLink(ctrl)
	gomock.InOrder(
		mockExec.EXPECT().LookPath(iptablesExecutable).Return("", nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "PREROUTING", preroutingRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", localhostTrafficFilterInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, testErr),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", blockIntrospectionOffhostAccessInputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().LookPath(ip6tablesExecutable).Return("", nil),
		mockExec.EXPECT().Command(ip6tablesExecutable,
			expectedArgs("filter", "-D", "INPUT", allowIntrospectionForDockerArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
		mockExec.EXPECT().Command(iptablesExecutable,
			expectedArgs("nat", "-D", "OUTPUT", outputRouteArgs)).Return(mockCmd),
		mockCmd.EXPECT().CombinedOutput().Return([]byte{0}, nil),
	)

	route, err := NewNetfilterRoute(mockExec, mockNetlink, testDockerBridgeInterfaceName)
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
	defer resetDefaultNetworkInterfaceVariables()
	defaultLoopbackInterfaceName = testLoopbackInterfaceName
	assert.Equal(t, []string{
		"INPUT",
		"-p", "tcp",
		"--dport", agentIntrospectionServerPort,
		"!", "-i", testLoopbackInterfaceName,
		"-j", "DROP",
	}, getBlockIntrospectionOffhostAccessInputChainArgs())
}

func TestAllowIntrospectionForDocker(t *testing.T) {
	defer resetDefaultNetworkInterfaceVariables()
	defaultDockerBridgeNetworkName = testDockerBridgeInterfaceName
	assert.Equal(t, []string{
		"INPUT",
		"-p", "tcp",
		"--dport", agentIntrospectionServerPort,
		"-i", testDockerBridgeInterfaceName,
		"-j", "ACCEPT",
	}, allowIntrospectionForDockerIptablesInputChainArgs())
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
