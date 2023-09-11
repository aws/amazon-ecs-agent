//go:build unit
// +build unit

package ecscni

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildNetworkConfig(t *testing.T) {
	nsPath := "nspath"
	cfg := &VPCTunnelConfig{
		CNIConfig: CNIConfig{
			NetNSPath: nsPath,
		},
	}

	rt := BuildRuntimeConfig(cfg)
	require.Equal(t, nsPath, rt.NetNS)
	require.Equal(t, defaultInterfaceName, rt.IfName)
	require.Equal(t, "container-id", rt.ContainerID)
}
