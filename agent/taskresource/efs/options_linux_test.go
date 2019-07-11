package efs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	optA := "hard,noresvport,timeo=24"
	optB := "hard,resvport,timeo=10,vers=5"
	merged := mergeOptions(defaultOptsForEFS, optA, optB)

	assert.Contains(t, merged, "timeo=10", "timeo should persist the right hand side value (10)")
	assert.Contains(t, merged, "resvport")
	assert.NotContains(t, merged, "noresvport")
	assert.Contains(t, merged, "vers=5")
}
