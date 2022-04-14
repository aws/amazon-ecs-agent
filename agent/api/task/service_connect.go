package task

// ServiceConnectConfig represents the Service Connect configuration for a task.
type ServiceConnectConfig struct {
	ContainerName string          `json:"containerName"`
	StatsConfig   StatsConfig     `json:"statsConfig"`
	IngressConfig []IngressConfig `json:"ingressConfig"`
	EgressConfig  EgressConfig    `json:"egressConfig"`
	DNSConfig     []DNSConfig     `json:"dnsConfig"`
}

// StatsConfig is the endpoint where SC stats can be retrieved from.
type StatsConfig struct {
	UrlPath string `json:"urlPath,omitempty"`
	Port    int    `json:"port,omitempty"`
}

// IngressConfig is the ingress configuration for a given SC service.
type IngressConfig struct {
	// ServiceName is the name of the SC service relevant to this IngressConfig.
	ServiceName string `json:"serviceName"`
	// ListenerName is the name of the listener for SC service with name ServiceName.
	ListenerName string `json:"listenerName"`
	// ListenerPort is the port where Envoy listens for ingress traffic for ServiceName.
	ListenerPort int `json:"listenerPort"`
	// InterceptPort is only relevant for awsvpc mode. If present, SC CNI Plugin will configure netfilter rules to redirect
	// traffic destined to this port to ListenerPort.
	InterceptPort *int `json:"interceptPort,omitempty"`
	// HostPort is only relevant for bridge network mode.
	HostPort *int `json:"hostPort,omitempty"`
}

// EgressConfig is the egress configuration for a given SC service.
type EgressConfig struct {
	// ListenerName is the name of the listener for SC service with name ServiceName.
	ListenerName string `json:"listenerName"`
	VIP          VIP    `json:"vip"`
}

// VIP is the representation of an SC VIP-CIDR
// e.g. 169.254.0.0/16
type VIP struct {
	IPV4 string `json:"ipv4,omitempty"`
	IPV6 string `json:"ipv6,omitempty"`
}

// DNSConfig represents a mapping between a VIP in the SC VIP-CIDR and an upstream SC service.
// e.g. DummySCService.corp.local -> 169.254.1.1
type DNSConfig struct {
	HostName    string `json:"hostName"`
	IPV4Address string `json:"ipv4Address,omitempty"`
	IPV6Address string `json:"ipv6Address,omitempty"`
}
