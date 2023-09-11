package ecscni

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
)

// AppMeshConfig contains the information needed to incoke the appmesh CNI plugin.
type AppMeshConfig struct {
	CNIConfig
	// IgnoredUID specifies egress traffic from the processes owned by the UID will be ignored
	IgnoredUID string `json:"ignoredUID,omitempty"`
	// IgnoredGID specifies egress traffic from the processes owned by the GID will be ignored
	IgnoredGID string `json:"ignoredGID,omitempty"`
	// ProxyIngressPort is the ingress port number that proxy is listening on
	ProxyIngressPort string `json:"proxyIngressPort"`
	// ProxyEgressPort is the egress port number that proxy is listening on
	ProxyEgressPort string `json:"proxyEgressPort"`
	// AppPorts specifies port numbers that application is listening on
	AppPorts []string `json:"appPorts"`
	// EgressIgnoredPorts is the list of ports for which egress traffic will be ignored
	EgressIgnoredPorts []string `json:"egressIgnoredPorts,omitempty"`
	// EgressIgnoredIPs is the list of IPs for which egress traffic will be ignored
	EgressIgnoredIPs []string `json:"egressIgnoredIPs,omitempty"`
}

func NewAppMeshConfig(cniConfig CNIConfig, cfg *appmesh.AppMesh) *AppMeshConfig {
	return &AppMeshConfig{
		CNIConfig:          cniConfig,
		IgnoredUID:         cfg.IgnoredUID,
		IgnoredGID:         cfg.IgnoredGID,
		ProxyIngressPort:   cfg.ProxyIngressPort,
		ProxyEgressPort:    cfg.ProxyEgressPort,
		AppPorts:           cfg.AppPorts,
		EgressIgnoredPorts: cfg.EgressIgnoredPorts,
		EgressIgnoredIPs:   cfg.EgressIgnoredIPs,
	}
}

func (amc *AppMeshConfig) String() string {
	return fmt.Sprintf("%s, ignored uid: %s, ignored gid: %s, ingress port: %s, "+
		"egress port: %s, app ports: %v, ignored egress ips: %v, ignored egress ports: %v",
		amc.CNIConfig.String(), amc.IgnoredUID, amc.IgnoredGID,
		amc.ProxyIngressPort, amc.ProxyEgressPort, amc.AppPorts,
		amc.EgressIgnoredIPs, amc.EgressIgnoredPorts)
}

func (amc *AppMeshConfig) InterfaceName() string {
	// Not required for app mesh plugin as no particular interface is set up in this
	// plugin. The plugin sets up some iptables filters, that's all. However, CNI requires
	// us to set it. Setting it to "eth0" just to satisfy that constraint.
	return "eth0"
}

func (amc *AppMeshConfig) NSPath() string {
	return amc.NetNSPath
}

func (amc *AppMeshConfig) PluginName() string {
	return amc.CNIPluginName
}

func (amc *AppMeshConfig) CNIVersion() string {
	return amc.CNISpecVersion
}
