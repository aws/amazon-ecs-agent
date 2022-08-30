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

package ecscni

import (
	"encoding/json"

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
)

// newNetworkConfig converts a network config to libcni's NetworkConfig.
func newNetworkConfig(netcfg interface{}, plugin string, cniVersion string) (*libcni.NetworkConfig, error) {
	configBytes, err := json.Marshal(netcfg)
	if err != nil {
		seelog.Errorf("[ECSCNI] Marshal configuration for plugin %s failed, error: %v", plugin, err)
		return nil, err
	}

	netConfig := &libcni.NetworkConfig{
		Network: &cnitypes.NetConf{
			Type:       plugin,
			CNIVersion: cniVersion,
			Name:       defaultNetworkName,
		},
		Bytes: configBytes,
	}

	return netConfig, nil
}
