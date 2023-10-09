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

package config

import (
	"errors"
)

// agentConfigAccessor struct implements AgentConfigAccessor interface defined in ecs-agent module.
type agentConfigAccessor struct {
	cfg *Config
}

// NewAgentConfigAccessor creates a new agentConfigAccessor.
func NewAgentConfigAccessor(cfg *Config) (*agentConfigAccessor, error) {
	if cfg == nil {
		return nil, errors.New("unable to create new agent config accessor due to passed in config value being nil")
	}

	return &agentConfigAccessor{cfg: cfg}, nil
}

func (aca *agentConfigAccessor) AcceptInsecureCert() bool {
	return aca.cfg.AcceptInsecureCert
}

func (aca *agentConfigAccessor) APIEndpoint() string {
	return aca.cfg.APIEndpoint
}

func (aca *agentConfigAccessor) AWSRegion() string {
	return aca.cfg.AWSRegion
}

func (aca *agentConfigAccessor) Cluster() string {
	return aca.cfg.Cluster
}

func (aca *agentConfigAccessor) DefaultClusterName() string {
	return DefaultClusterName
}

func (aca *agentConfigAccessor) External() bool {
	return aca.cfg.External.Enabled()
}

func (aca *agentConfigAccessor) InstanceAttributes() map[string]string {
	return aca.cfg.InstanceAttributes
}

func (aca *agentConfigAccessor) NoInstanceIdentityDocument() bool {
	return aca.cfg.NoIID
}

func (aca *agentConfigAccessor) OSFamily() string {
	return GetOSFamily()
}

func (aca *agentConfigAccessor) OSType() string {
	return OSType
}

func (aca *agentConfigAccessor) ReservedMemory() uint16 {
	return aca.cfg.ReservedMemory
}

func (aca *agentConfigAccessor) ReservedPorts() []uint16 {
	return aca.cfg.ReservedPorts
}

func (aca *agentConfigAccessor) ReservedPortsUDP() []uint16 {
	return aca.cfg.ReservedPortsUDP
}

func (aca *agentConfigAccessor) UpdateCluster(cluster string) {
	aca.cfg.Cluster = cluster
}
