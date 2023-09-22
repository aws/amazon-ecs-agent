//go:build unit
// +build unit

package uuid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateUuidWithPrefix(t *testing.T) {
	for _, testCase := range []struct {
		prefix         string
		separator      string
		expectedPrefix string
	}{
		{
			prefix:         "prefix",
			separator:      DefaultSeparator,
			expectedPrefix: "prefix-",
		},
		{
			prefix:         "prefix",
			separator:      "+",
			expectedPrefix: "prefix+",
		},
		{
			prefix:         "prefix",
			separator:      "",
			expectedPrefix: "prefix",
		},
		{
			prefix:         "",
			separator:      DefaultSeparator,
			expectedPrefix: "",
		},
		{
			prefix:         "",
			separator:      "",
			expectedPrefix: "",
		},
	} {
		generatedUuid := GenerateWithPrefix(testCase.prefix, testCase.separator)
		assert.True(t, strings.HasPrefix(generatedUuid, testCase.expectedPrefix))
	}
}
