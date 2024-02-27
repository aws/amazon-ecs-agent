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

package tasknetworkconfig

import ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

const (
	primaryNetNSName       = "primary-netns"
	primaryNetNSPath       = "primary-path"
	secondaryNetNSName     = "secondary-netns"
	primaryInterfaceName   = "primary-interface"
	secondaryInterfaceName = "secondary-interface"
)

func getTestTaskNetworkConfig() *TaskNetworkConfig {
	return &TaskNetworkConfig{
		NetworkNamespaces: getTestNetworkNamespaces(),
	}
}

func getTestNetworkNamespaces() []*NetworkNamespace {
	return []*NetworkNamespace{
		{
			Name:              secondaryNetNSName,
			Index:             1,
			NetworkInterfaces: getTestNetworkInterfaces(),
		},
		{
			Name:              primaryNetNSName,
			Index:             0,
			NetworkInterfaces: getTestNetworkInterfaces(),
		},
	}
}

func getTestNetworkInterfaces() []*ni.NetworkInterface {
	return []*ni.NetworkInterface{
		{
			Name:    secondaryInterfaceName,
			Default: false,
			Index:   1,
		},
		{
			Name:    primaryInterfaceName,
			Default: true,
			Index:   0,
		},
	}
}
