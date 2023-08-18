//go:build windows && unit
// +build windows,unit

package resource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVolumeID = "vol-0a1234f3403277890"
)

func Test_parseExecutableOutput_HappyPath(t *testing.T) {
	output := fmt.Sprintf("SerialNumber \n	%s_00000001.", testVolumeID)
	parsedOutput, err := parseExecutableOutput([]byte(output))
	require.NoError(t, err)
	assert.True(t, strings.Contains(parsedOutput, testVolumeID))
}

func Test_parseExecutableOutput_UnexpectedOutput(t *testing.T) {
	output := "No Instance(s) Available."
	parsedOutput, err := parseExecutableOutput([]byte(output))
	require.Error(t, err, "cannot find the volume ID: %s", output)
	assert.Equal(t, "", parsedOutput)
}
