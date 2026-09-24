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

package netlib

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	mock_data "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_platform "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform/mocks"

	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const extendTaskID = "task-extend"

// provisionedNamespace models a namespace built ahead of the payload and
// driven through the pull phase: one interface, both at READY_PULL.
func provisionedNamespace(t *testing.T) *tasknetworkconfig.NetworkNamespace {
	t.Helper()
	iface := &networkinterface.NetworkInterface{ID: "eni-primary", Index: 0, Default: true, DeviceName: "eth1",
		KnownStatus: status.NetworkReadyPull, DesiredStatus: status.NetworkReadyPull}
	netNS, err := tasknetworkconfig.NewNetworkNamespace("ns-primary", "/var/run/netns/ns-primary", 0, nil, iface)
	require.NoError(t, err)
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReadyPull
	return netNS
}

// rebuilt is what the platform returns for the same namespace: fresh state,
// the held interface plus whatever the payload added.
func rebuilt(t *testing.T, added ...*networkinterface.NetworkInterface) *tasknetworkconfig.TaskNetworkConfig {
	t.Helper()
	ifaces := []*networkinterface.NetworkInterface{{ID: "eni-primary", Index: 0, Default: true, DeviceName: "eth1"}}
	ifaces = append(ifaces, added...)
	netNS, err := tasknetworkconfig.NewNetworkNamespace("ns-primary", "/var/run/netns/ns-primary", 0, nil, ifaces...)
	require.NoError(t, err)
	return &tasknetworkconfig.TaskNetworkConfig{NetworkMode: types.NetworkModeAwsvpc,
		NetworkNamespaces: []*tasknetworkconfig.NetworkNamespace{netNS}}
}

func newExtendTestBuilder(ctrl *gomock.Controller) (*networkBuilder, *mock_platform.MockAPI, *mock_data.MockNetworkDataClient) {
	platformAPI := mock_platform.NewMockAPI(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	entry := mock_metrics.NewMockEntry(ctrl)
	metricsFactory.EXPECT().New(gomock.Any()).Return(entry).AnyTimes()
	entry.EXPECT().WithFields(gomock.Any()).Return(entry).AnyTimes()
	entry.EXPECT().Done(gomock.Any()).AnyTimes()
	netDao := mock_data.NewMockNetworkDataClient(ctrl)
	return &networkBuilder{platformAPI: platformAPI, metricsFactory: metricsFactory, networkDAO: netDao}, platformAPI, netDao
}

// TestExtendCarriesStateWithoutAdditions verifies a payload that adds nothing
// yields the existing namespace's state on the rebuilt model and touches the
// host not at all.
func TestExtendCarriesStateWithoutAdditions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	nb, platformAPI, _ := newExtendTestBuilder(ctrl)

	existing := provisionedNamespace(t)
	payload := &ecsacs.Task{}
	platformAPI.EXPECT().ExtendTaskNetworkConfiguration(extendTaskID, payload, []*tasknetworkconfig.NetworkNamespace{existing}).
		Return(rebuilt(t), nil)

	cfg, err := nb.ExtendTaskNetworkConfiguration(context.Background(), extendTaskID, payload, existing)
	require.NoError(t, err)
	got := cfg.NetworkNamespaces[0]
	require.NotSame(t, existing, got)
	require.Equal(t, status.NetworkReadyPull, got.KnownState)
	require.Equal(t, status.NetworkReadyPull, got.DesiredState)
	require.Equal(t, status.NetworkReadyPull, got.NetworkInterfaces[0].KnownStatus)
	require.Equal(t, status.NetworkReadyPull, existing.KnownState, "the existing model is not modified")
}

// TestExtendConfiguresAdditionsThroughPullPhase verifies an added interface is
// configured through the pull phase against the existing namespace, while the
// interface already configured is left alone, and the namespace ends at the
// state it started in.
func TestExtendConfiguresAdditionsThroughPullPhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	nb, platformAPI, netDao := newExtendTestBuilder(ctrl)

	existing := provisionedNamespace(t)
	payload := &ecsacs.Task{}
	added := &networkinterface.NetworkInterface{ID: "eni-second", Index: 1, DeviceName: "eth2",
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol}
	platformAPI.EXPECT().ExtendTaskNetworkConfiguration(extendTaskID, payload, gomock.Any()).Return(rebuilt(t, added), nil)

	// The pull phase for a namespace already at READY_PULL: no netns or DNS
	// creation, one interface configured, and it is the addition.
	platformAPI.EXPECT().ConfigureInterface(gomock.Any(), "/var/run/netns/ns-primary", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, iface *networkinterface.NetworkInterface, _ interface{}) error {
			require.Equal(t, "eni-second", iface.ID)
			require.Equal(t, status.NetworkReadyPull, iface.DesiredStatus)
			return nil
		}).Times(1)
	netDao.EXPECT().SaveNetworkNamespace(gomock.Any()).Return(nil).AnyTimes()

	cfg, err := nb.ExtendTaskNetworkConfiguration(context.Background(), extendTaskID, payload, existing)
	require.NoError(t, err)
	got := cfg.NetworkNamespaces[0]
	require.Equal(t, status.NetworkReadyPull, got.KnownState)
	require.Equal(t, status.NetworkReadyPull, got.DesiredState, "the namespace's desired state is restored after the pass")
	require.Len(t, got.NetworkInterfaces, 2)
	require.Equal(t, status.NetworkReadyPull, got.GetInterfaceByIndex(0).KnownStatus)
	require.Equal(t, status.NetworkReadyPull, got.GetInterfaceByIndex(1).KnownStatus, "the addition passed the pull phase")
	require.Len(t, existing.NetworkInterfaces, 1, "the existing model is not modified")
}

// TestExtendReportsFailures verifies platform build errors and pull-phase
// errors are returned rather than yielding a half-configured namespace.
func TestExtendReportsFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	nb, platformAPI, _ := newExtendTestBuilder(ctrl)
	existing := provisionedNamespace(t)
	payload := &ecsacs.Task{}

	platformAPI.EXPECT().ExtendTaskNetworkConfiguration(extendTaskID, payload, gomock.Any()).
		Return(nil, errors.New("undeclared interface"))
	_, err := nb.ExtendTaskNetworkConfiguration(context.Background(), extendTaskID, payload, existing)
	require.ErrorContains(t, err, "undeclared interface")

	added := &networkinterface.NetworkInterface{ID: "eni-second", Index: 1,
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol}
	platformAPI.EXPECT().ExtendTaskNetworkConfiguration(extendTaskID, payload, gomock.Any()).Return(rebuilt(t, added), nil)
	platformAPI.EXPECT().ConfigureInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("cni add failed"))
	_, err = nb.ExtendTaskNetworkConfiguration(context.Background(), extendTaskID, payload, existing)
	require.ErrorContains(t, err, "cni add failed")
}
