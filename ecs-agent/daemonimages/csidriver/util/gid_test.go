//go:build unit
// +build unit

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateGIDFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name: "empty path",
			path: "",
		},
		{
			name: "simple path",
			path: "/dev/xvda",
		},
		{
			name: "same path should generate same GID",
			path: "/dev/xvda",
		},
	}

	// Store GIDs to check for consistency
	gidMap := make(map[string]int)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gid := GenerateGIDFromPath(tc.path)

			// Check that GID is in the valid range
			assert.GreaterOrEqual(t, gid, minGID)
			assert.LessOrEqual(t, gid, maxGID)

			// If we've seen this path before, check that the GID is the same
			if prevGID, ok := gidMap[tc.path]; ok && tc.path != "" {
				assert.Equal(t, prevGID, gid, "Same path should generate same GID")
			}

			// Store the GID for this path
			gidMap[tc.path] = gid
		})
	}

	// Test that different paths generate different GIDs (with high probability)
	path1 := "/dev/xvda"
	path2 := "/dev/xvdb"
	gid1 := GenerateGIDFromPath(path1)
	gid2 := GenerateGIDFromPath(path2)
	assert.NotEqual(t, gid1, gid2, "Different paths should generate different GIDs")
}
