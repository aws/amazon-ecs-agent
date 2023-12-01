//go:build !windows && unit
// +build !windows,unit

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

package netlib

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	mock_platform "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform/mocks"
	mock_netwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNewNetworkBuilder(t *testing.T) {
	nbi, err := NewNetworkBuilder(platform.WarmpoolPlatform, nil, nil, data.Client{}, "")
	nb := nbi.(*networkBuilder)
	require.NoError(t, err)
	require.NotNil(t, nb.platformAPI)

	nbi, err = NewNetworkBuilder("invalid-platform", nil, nil, data.Client{}, "")
	require.Error(t, err)
	require.Nil(t, nbi)
}

// TestNetworkBuilder_BuildTaskNetworkConfiguration verifies for all known use cases,
// the network builder is able to translate the input task payload into the desired
// network data models.
func TestNetworkBuilder_BuildTaskNetworkConfiguration(t *testing.T) {
	// Warmpool test cases.
	t.Run("containerd-default", getTestFunc(getSingleNetNSAWSVPCTestData, platform.WarmpoolPlatform))
	t.Run("containerd-multi-interface", getTestFunc(getSingleNetNSMultiIfaceAWSVPCTestData, platform.WarmpoolPlatform))
	t.Run("containerd-multi-netns", getTestFunc(getMultiNetNSMultiIfaceAWSVPCTestData, platform.WarmpoolPlatform))

	// Firecracker test cases.
	t.Run("firecracker-v2n-veth", getTestFunc(getV2NTestData, platform.FirecrackerPlatform))
}

func TestNetworkBuilder_Start(t *testing.T) {
	t.Run("awsvpc", testNetworkBuilder_StartAWSVPC)
}

// TestNetworkBuilder_Stop verifies stop workflow for AWSVPC mode.
func TestNetworkBuilder_Stop(t *testing.T) {
	t.Run("awsvpc", testNetworkBuilder_StopAWSVPC)
}

// getTestFunc returns a test function that verifies the capability of the networkBuilder
// to translate a given input task payload into desired network data models.
func getTestFunc(
	dataGenF func(string) (input *ecsacs.Task, expected tasknetworkconfig.TaskNetworkConfig),
	plt string,
) func(*testing.T) {

	return func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a networkBuilder for the warmpool platform.
		mockNet := mock_netwrapper.NewMockNet(ctrl)
		platformAPI, err := platform.NewPlatform(plt, nil, "", mockNet)
		require.NoError(t, err)
		netBuilder := &networkBuilder{
			platformAPI: platformAPI,
		}

		// Generate input task payload and a reference to verify the output with.
		taskPayload, expectedConfig := dataGenF(taskID)

		// The agent expects the regular / trunk ENI to be present on the host.
		// We should mock the net.Interfaces() method to return a list of interfaces
		// on the host accordingly.
		var ifaces []net.Interface
		idx := 1
		for _, eni := range taskPayload.ElasticNetworkInterfaces {
			var mac string
			// In case of regular ENIs, the agent expects to find an interface with
			// the ACS ENI's MAC address on the host. In case of branch ENIs (which
			// use VLAN ID), the agent expects to find a trunk interface with the MAC
			// address specified in the VLAN properties of the ACS ENI.
			if eni.InterfaceVlanProperties == nil {
				mac = aws.StringValue(eni.MacAddress)
			} else {
				mac = aws.StringValue(eni.InterfaceVlanProperties.TrunkInterfaceMacAddress)
			}
			// Veth and V2N interfaces will not have a MAC address associated with them.
			if mac == "" {
				continue
			}
			hw, err := net.ParseMAC(mac)
			require.NoError(t, err)
			ifaces = append(ifaces, net.Interface{
				HardwareAddr: hw,
				Name:         fmt.Sprintf("eth%d", idx),
			})
			idx += 1
		}
		mockNet.EXPECT().Interfaces().Return(ifaces, nil).Times(1)

		// Invoke networkBuilder function for building the task network config.
		actualConfig, err := netBuilder.BuildTaskNetworkConfiguration(taskID, taskPayload)
		require.NoError(t, err)

		// Convert the obtained output and the reference data into json data to make it
		// easier to compare.
		expected, err := json.Marshal(expectedConfig)
		require.NoError(t, err)
		actual, err := json.Marshal(actualConfig)
		require.NoError(t, err)

		require.Equal(t, string(expected), string(actual))
	}
}

// testNetworkBuilder_StartAWSVPC verifies that the expected platform API calls
// are made by the network builder while configuring each network namespace.
// The test includes all known network configuration for a netns in AWSVPC mode.
func testNetworkBuilder_StartAWSVPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	platformAPI := mock_platform.NewMockAPI(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	netBuilder := &networkBuilder{
		platformAPI:    platformAPI,
		metricsFactory: metricsFactory,
	}

	// Single ENI use case without AppMesh and service connect configs.
	_, taskNetConfig := getSingleNetNSAWSVPCTestData(taskID)
	netNS := taskNetConfig.GetPrimaryNetNS()
	require.NotNil(t, netNS)

	netNS.KnownState = status.NetworkNone
	netNS.DesiredState = status.NetworkReadyPull
	t.Run("single-eni-default", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with AppMesh config and desired state = READY_PULL.
	// In this case, the appmesh configuration should not be executed.
	netNS.AppMeshConfig = &appmesh.AppMesh{
		// Placeholder data.
		ContainerName: "appmesh-envoy",
	}
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("single-eni-appmesh-readypull", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with AppMesh config and desired state = READY.
	// The appmesh configuration should get executed now.
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("single-eni-appmesh-ready", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with ServiceConnect and desired state = READY.
	// In this case, the ServiceConnect configuration should be executed.
	netNS.AppMeshConfig = nil
	netNS.ServiceConnectConfig = &serviceconnect.ServiceConnectConfig{
		ServiceConnectContainerName: "ecs-service-connect",
	}
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("single-eni-serviceconnect-ready", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with ServiceConnect and desired state = READY_PULL.
	// In this case, the ServiceConnect configuration should not be executed.
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("single-eni-serviceconnect-readypull", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single netns with multi interface case.
	_, taskNetConfig = getSingleNetNSMultiIfaceAWSVPCTestData(taskID)
	netNS = taskNetConfig.GetPrimaryNetNS()
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("multi-eni-default", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Desired state = DELETED. There should be no expected calls to
	// platform APIs for this case.
	netNS.DesiredState = status.NetworkDeleted
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("deleted", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})
}

// getExpectedCalls_StartAWSVPC takes a netns configuration as input and
// generates a list of expected `gomock` calls to the mock platform API
// which the network builder will invoke during the test runs. The calls
// generated will specifically used to test the `Start` method of the
// network builder.
func getExpectedCalls_StartAWSVPC(
	ctx context.Context,
	platformAPI *mock_platform.MockAPI,
	metricsFactory *mock_metrics.MockEntryFactory,
	mockEntry *mock_metrics.MockEntry,
	netNS *tasknetworkconfig.NetworkNamespace,
) []*gomock.Call {
	var calls []*gomock.Call

	calls = append(calls,
		metricsFactory.EXPECT().New(metrics.BuildNetworkNamespaceMetricName).Return(mockEntry).Times(1),
		mockEntry.EXPECT().WithFields(gomock.Any()).Return(mockEntry).Times(1))

	// Start() should not be invoked when desired state = DELETED.
	if netNS.DesiredState == status.NetworkDeleted {
		calls = append(calls,
			mockEntry.EXPECT().Done(gomock.Any()).Times(1))
		return calls
	}

	// Network namespace creation and DNS config files creation is to happen
	// only while transitioning from NONE to READY_PULL.
	if netNS.KnownState == status.NetworkNone &&
		netNS.DesiredState == status.NetworkReadyPull {
		calls = append(calls,
			platformAPI.EXPECT().CreateNetNS(netNS.Path).Return(nil).Times(1),
			platformAPI.EXPECT().CreateDNSConfig(taskID, netNS).Return(nil).Times(1))
	}

	// For each interface inside the netns, the network builder needs to invoke the
	// `ConfigureInterface` platformAPI.
	for _, iface := range netNS.NetworkInterfaces {
		if iface.KnownStatus == netNS.DesiredState {
			continue
		}
		calls = append(calls,
			platformAPI.EXPECT().ConfigureInterface(ctx, netNS.Path, iface).Return(nil).Times(1))
	}

	// AppMesh/ServiceConnect configurations are executed only during the READY_PULL -> READY transitions.
	if netNS.KnownState == status.NetworkReadyPull &&
		netNS.DesiredState == status.NetworkReady {
		// AppMesh and ServiceConnect configuration happens only if the configuration data is present.
		if netNS.AppMeshConfig != nil {
			calls = append(calls, platformAPI.EXPECT().ConfigureAppMesh(ctx, netNS.Path, netNS.AppMeshConfig).
				Return(nil).Times(1))
		}
		if netNS.ServiceConnectConfig != nil {
			calls = append(calls, platformAPI.EXPECT().ConfigureServiceConnect(ctx, netNS.Path,
				netNS.GetPrimaryInterface(), netNS.ServiceConnectConfig).Return(nil).Times(1))
		}
	}

	calls = append(calls, mockEntry.EXPECT().Done(nil).Times(1))

	return calls
}

func testNetworkBuilder_StopAWSVPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	platformAPI := mock_platform.NewMockAPI(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	netBuilder := &networkBuilder{
		platformAPI:    platformAPI,
		metricsFactory: metricsFactory,
	}

	// Single ENI use case without AppMesh and service connect configs.
	_, taskNetConfig := getSingleNetNSMultiIfaceAWSVPCTestData(taskID)
	netNS := taskNetConfig.GetPrimaryNetNS()
	mockEntry = mock_metrics.NewMockEntry(ctrl)
	t.Run("multi-eni", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StopAWSVPC(ctx, platformAPI, metricsFactory, mockEntry, netNS)...,
		)
		netBuilder.Stop(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})
}

// getExpectedCalls_StopAWSVPC returns a list of gomock calls that will be executed
// while test stop workflow for AWSVPC.
func getExpectedCalls_StopAWSVPC(
	ctx context.Context,
	platformAPI *mock_platform.MockAPI,
	metricsFactory *mock_metrics.MockEntryFactory,
	mockEntry *mock_metrics.MockEntry,
	netNS *tasknetworkconfig.NetworkNamespace,
) []*gomock.Call {
	var calls []*gomock.Call

	calls = append(calls,
		metricsFactory.EXPECT().New(metrics.DeleteNetworkNamespaceMetricName).Return(mockEntry).Times(1),
		mockEntry.EXPECT().WithFields(gomock.Any()).Return(mockEntry).Times(1))

	// Stop() should not be invoked when desired state = DELETED.
	if netNS.DesiredState != status.NetworkDeleted {
		calls = append(calls,
			mockEntry.EXPECT().Done(gomock.Any()).Times(1))
		return calls
	}

	// For each interface inside the netns, the network builder needs to invoke the
	// `ConfigureInterface` platformAPI.
	for _, iface := range netNS.NetworkInterfaces {
		if iface.KnownStatus == netNS.DesiredState {
			continue
		}
		calls = append(calls,
			platformAPI.EXPECT().ConfigureInterface(ctx, netNS.Path, iface).Return(nil).Times(1))
	}

	return append(calls,
		platformAPI.EXPECT().DeleteNetNS(netNS.Path).Return(nil).Times(1))
}
