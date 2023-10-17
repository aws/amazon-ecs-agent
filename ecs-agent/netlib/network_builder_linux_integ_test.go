//go:build !windows && integration
// +build !windows,integration

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
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"
	"github.com/aws/aws-sdk-go/aws"
	"net"
	"os"

	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func TestNetworkBuilder_AWSVPC(t *testing.T) {
	workDir, err := os.Getwd()
	require.NoError(t, err)
	cniSearchPath := workDir + "/../../out/cni-plugins"

	platformAPI, err := platform.NewPlatform(
		platform.WarmpoolPlatform,
		ecscni.NewNetNSUtil(),
		volume.NewTmpAccessor(t.Name()),
		oswrapper.NewOS(),
		ioutilwrapper.NewIOUtil(),
		netlinkwrapper.New(),
		"/tmp/stateDBDirectory",
		ecscni.NewCNIClient([]string{cniSearchPath}))
	require.NoError(t, err)

	netBuilder := &networkBuilder{
		platformAPI: platformAPI,
	}

	t.Run("single-eni", awsvpcSingleENITestFunc(netBuilder))
}

func awsvpcSingleENITestFunc(netBuilder *networkBuilder) func(*testing.T) {
	return func(t *testing.T) {
		taskPayload, _ := getSingleNetNSAWSVPCTestData(taskID)

		// Create a dummy interface in the default network namespace
		// mimicking the presence of the task ENI.
		primaryIF := taskPayload.ElasticNetworkInterfaces[0]
		mac := aws.StringValue(primaryIF.MacAddress)
		dummyENI, err := createDummyInterface("eth1-test", mac)
		require.NoError(t, err)
		// Delete dummy interface after test is complete.
		defer deleteDummyInterface(dummyENI)

		// Create task network configuration from test task payload.
		taskNetConfig, err := netBuilder.BuildTaskNetworkConfiguration(taskID, taskPayload)
		require.NoError(t, err)

		netNS := taskNetConfig.GetPrimaryNetNS()
		err = netBuilder.Start(context.TODO(), ecs.NetworkModeAwsvpc, taskID, netNS)
		require.NoError(t, err)
	}
}

func createDummyInterface(name, mac string) (netlink.Link, error) {
	ha, err := net.ParseMAC(mac)
	if err != nil {
		return nil, err
	}

	la := netlink.NewLinkAttrs()
	la.Name = name
	la.HardwareAddr = ha
	link := &netlink.Dummy{LinkAttrs: la}
	err = netlink.LinkAdd(link)
	if err != nil {
		return nil, err
	}

	return link, nil
}

func deleteDummyInterface(link netlink.Link) error {
	return netlink.LinkDel(link)
}
