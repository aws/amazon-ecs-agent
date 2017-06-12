package pause

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedPlatform(t *testing.T) {
	testCases := map[error]bool{
		errors.New("error"):                           false,
		UnsupportedPlatformError{errors.New("error")}: true,
	}

	for err, expected := range testCases {
		t.Run(fmt.Sprintf("returns %t for type %s", expected, reflect.TypeOf(err)), func(t *testing.T) {
			assert.Equal(t, expected, UnsupportedPlatform(err))
		})
	}
}
