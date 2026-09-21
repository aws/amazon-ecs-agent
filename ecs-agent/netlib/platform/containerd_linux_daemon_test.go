//go:build unit && linux
// +build unit,linux

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

package platform

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	mock_nsutil "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testDaemonNetNSPath = "/var/run/netns/host-daemon"
)

// daemonNetNS returns a daemon-bridge netns fixture in the given state.
func daemonNetNS(known, desired status.NetworkStatus) *tasknetworkconfig.NetworkNamespace {
	return &tasknetworkconfig.NetworkNamespace{
		Name:         "host-daemon",
		Path:         testDaemonNetNSPath,
		NetworkMode:  "daemon-bridge",
		KnownState:   known,
		DesiredState: desired,
		NetworkInterfaces: []*networkinterface.NetworkInterface{{
			Name:           "host-daemon",
			Default:        true,
			PrivateDNSName: "daemon",
			IPV4Addresses:  []*networkinterface.IPV4Address{{Primary: true, Address: "169.254.172.2"}},
		}},
	}
}

// expectDaemonDNSFiles registers the file writes of the create phase: the
// namespace's resolv.conf, hostname and hosts, plus the host /etc/hostname
// existence check. Returns the wrappers to install on the platform.
func expectDaemonDNSFiles(ctrl *gomock.Controller, nsUtil *mock_nsutil.MockNetNSUtil) (*mock_oswrapper.MockOS, *mock_ioutilwrapper.MockIOUtil) {
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockOS.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()
	mockOS.EXPECT().IsNotExist(gomock.Any()).Return(false).AnyTimes()
	mockOS.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockFile, nil).AnyTimes()
	mockFile.EXPECT().Close().Return(nil).AnyTimes()
	nsUtil.EXPECT().BuildResolvConfig(gomock.Any(), gomock.Any()).Return("").AnyTimes()
	mockIOUtil.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	return mockOS, mockIOUtil
}

// TestContainerdConfigureDaemonNetNSCreatePhase verifies the NONE ->
// READY_PULL transition: the namespace is created (so that its existence is
// observable by concurrent bridge configuration), and no CNI plugin runs.
func TestContainerdConfigureDaemonNetNSCreatePhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
	mockCNI := mock_ecscni2.NewMockCNI(ctrl)

	// The namespace is created; creation is idempotent via the exists check.
	nsUtil.EXPECT().GetNetNSPath("host-daemon").Return(testDaemonNetNSPath).AnyTimes()
	nsUtil.EXPECT().NSExists(testDaemonNetNSPath).Return(false, nil).Times(1)
	nsUtil.EXPECT().NewNetNS(testDaemonNetNSPath).Return(nil).Times(1)
	nsUtil.EXPECT().ExecInNSPath(testDaemonNetNSPath, gomock.Any()).Return(nil).Times(1)
	// No CNI plugin execution in the create phase: mockCNI has no expected calls.
	mockOS, mockIOUtil := expectDaemonDNSFiles(ctrl, nsUtil)

	c := &containerd{
		common: common{
			nsUtil:    nsUtil,
			cniClient: mockCNI,
			os:        mockOS,
			ioutil:    mockIOUtil,
		},
	}

	err := c.ConfigureDaemonNetNS(daemonNetNS(status.NetworkNone, status.NetworkReadyPull))
	assert.NoError(t, err)
}

// TestContainerdConfigureDaemonNetNSCreatePhaseIdempotent verifies that a
// re-run of the create phase against an existing namespace is a no-op.
func TestContainerdConfigureDaemonNetNSCreatePhaseIdempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
	nsUtil.EXPECT().GetNetNSPath("host-daemon").Return(testDaemonNetNSPath).AnyTimes()
	nsUtil.EXPECT().NSExists(testDaemonNetNSPath).Return(true, nil).Times(1)
	// No NewNetNS, no ExecInNSPath, no CNI. The DNS files are still
	// (re)written: they are what a daemon container mounts.
	mockOS, mockIOUtil := expectDaemonDNSFiles(ctrl, nsUtil)

	c := &containerd{
		common: common{nsUtil: nsUtil, os: mockOS, ioutil: mockIOUtil},
	}

	err := c.ConfigureDaemonNetNS(daemonNetNS(status.NetworkNone, status.NetworkReadyPull))
	assert.NoError(t, err)
}

// TestContainerdConfigureDaemonNetNSConfigurePhase verifies the READY_PULL ->
// READY transition: the configured guard is consulted, CNI plugin env vars
// are set, and the namespace is attached to the bridge via the CNI plugins.
// The configuration must not re-create the namespace.
func TestContainerdConfigureDaemonNetNSConfigurePhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
	mockCNI := mock_ecscni2.NewMockCNI(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)

	// The configured guard inspects the namespace; an error means "not
	// configured yet", so configuration proceeds.
	nsUtil.EXPECT().ExecInNSPath(testDaemonNetNSPath, gomock.Any()).
		Return(fmt.Errorf("eth0 interface not found in daemon namespace")).Times(1)
	// CNI plugin environment (log file, IPAM db path).
	mockOS.EXPECT().Setenv(gomock.Any(), gomock.Any()).Times(2)
	// The daemon-bridge plugin config executes as a single Add of the
	// bridge+ipam plugin pair. No NewNetNS: configuration never creates.
	// The config must claim the daemon's static address rather than
	// allocate one dynamically.
	var captured ecscni.PluginConfig
	mockCNI.EXPECT().Add(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, cfg ecscni.PluginConfig) (interface{}, error) {
			captured = cfg
			return nil, nil
		}).MinTimes(1)

	c := &containerd{
		common: common{
			nsUtil:    nsUtil,
			cniClient: mockCNI,
			os:        mockOS,
		},
	}

	err := c.ConfigureDaemonNetNS(daemonNetNS(status.NetworkReadyPull, status.NetworkReady))
	assert.NoError(t, err)

	bridgeConfig, ok := captured.(*ecscni.BridgeConfig)
	assert.True(t, ok, "daemon configuration must execute a bridge plugin config")
	assert.Equal(t, DaemonBridgeIP, bridgeConfig.IPAM.IPV4Address,
		"daemon must claim its static bridge address, never allocate dynamically")
	assert.Equal(t, DaemonBridgeGatewayIP, bridgeConfig.IPAM.IPV4Gateway)
	assert.Equal(t, BridgeInterfaceName, bridgeConfig.Name)
}

// TestContainerdConfigureDaemonNetNSInvalidTransition verifies that a
// DELETED desired state is rejected, mirroring the managed platform.
func TestContainerdConfigureDaemonNetNSInvalidTransition(t *testing.T) {
	c := &containerd{common: common{}}

	err := c.ConfigureDaemonNetNS(daemonNetNS(status.NetworkNone, status.NetworkDeleted))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid transition state")
}

// TestContainerdConfigureDaemonNetNSCreateFailure verifies that a namespace
// creation failure is returned to the caller (the caller treats the daemon
// namespace as best-effort; this platform method just reports).
func TestContainerdConfigureDaemonNetNSCreateFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
	nsUtil.EXPECT().GetNetNSPath("host-daemon").Return(testDaemonNetNSPath).AnyTimes()
	nsUtil.EXPECT().NSExists(testDaemonNetNSPath).Return(false, nil).Times(1)
	nsUtil.EXPECT().NewNetNS(testDaemonNetNSPath).Return(fmt.Errorf("mount failed")).Times(1)

	c := &containerd{common: common{nsUtil: nsUtil}}

	err := c.ConfigureDaemonNetNS(daemonNetNS(status.NetworkNone, status.NetworkReadyPull))
	assert.Error(t, err)
}
