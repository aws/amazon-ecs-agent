package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

// Tests that NodeGetCapabilities returns the node's capabilities
func TestNodeGetCapabilities(t *testing.T) {
	node := &nodeService{}
	res, err := node.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	assert.NoError(t, err)

	capTypes := []csi.NodeServiceCapability_RPC_Type{}
	for _, cap := range res.Capabilities {
		capTypes = append(capTypes, cap.GetRpc().GetType())
	}

	expectedCapTypes := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	}
	assert.Equal(t, len(expectedCapTypes), len(capTypes))
	for _, expectedCapType := range expectedCapTypes {
		assert.Contains(t, capTypes, expectedCapType)
	}
}
