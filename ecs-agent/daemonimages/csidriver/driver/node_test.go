//go:build unit
// +build unit

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

package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that NodeGetCapabilities returns the node's capabilities
func TestNodeGetCapabilities(t *testing.T) {
	node := &nodeService{}
	res, err := node.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	require.NoError(t, err)

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
