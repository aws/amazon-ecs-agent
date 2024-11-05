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

//go:build windows && unit
// +build windows,unit

package netlib

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	mock_netwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNetworkBuilder_BuildTaskNetworkConfiguration(t *testing.T) {
	t.Run("containerd-default", getTestFunc(getSingleNetNSAWSVPCTestData, platform.WarmpoolPlatform))
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
		config := platform.Config{Name: plt}
		platformAPI, err := platform.NewPlatform(config, nil, "", mockNet)
		require.NoError(t, err)
		netBuilder := &networkBuilder{
			platformAPI: platformAPI,
		}

		// Generate input task payload and a reference to verify the output with.
		taskPayload, expectedConfig := dataGenF(taskID)

		// The agent expects the regular ENI to be present on the host.
		// We should mock the net.Interfaces() method to return a list of interfaces
		// on the host accordingly.
		var ifaces []net.Interface
		idx := 1
		for _, eni := range taskPayload.ElasticNetworkInterfaces {
			mac := aws.StringValue(eni.MacAddress)
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

		// NetNS path on windows is auto generated in place. Hence, we exclude it from verification.
		for i := 0; i < len(actualConfig.NetworkNamespaces); i++ {
			expectedConfig.NetworkNamespaces[i].Path = actualConfig.NetworkNamespaces[i].Path
		}
		// Convert the obtained output and the reference data into json data to make it
		// easier to compare.
		expected, err := json.Marshal(expectedConfig)
		require.NoError(t, err)
		actual, err := json.Marshal(actualConfig)
		require.NoError(t, err)

		require.Equal(t, string(expected), string(actual))
	}
}
