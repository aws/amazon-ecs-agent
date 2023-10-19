//go:build !windows && unit
// +build !windows,unit

package platform

import (
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestNewPlatform(t *testing.T) {
	_, err := NewPlatform(WarmpoolPlatform, nil)
	assert.NoError(t, err)

	_, err = NewPlatform("invalid-platform", nil)
	assert.Error(t, err)
}
