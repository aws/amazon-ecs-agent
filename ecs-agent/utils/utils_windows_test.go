package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestGetNumCPU(t *testing.T) {
	testCases := []struct {
		win32APIReturn      uint32
		runtimeNumCPUReturn int
		expectedAnswer      int
		name                string
	}{
		{128, 64, 128, "Both are valid"},
		{0, 64, 64, "win32 API invalid"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				win32APIGetAllActiveProcessorCount = windows.GetActiveProcessorCount
				golangRuntimeNumCPU = runtime.NumCPU
			}()

			win32APIGetAllActiveProcessorCount = func(groupNumber uint16) (ret uint32) {
				return tc.win32APIReturn
			}

			golangRuntimeNumCPU = func() int {
				return tc.runtimeNumCPUReturn
			}

			assert.Equal(t, tc.expectedAnswer, GetNumCPU(), tc.name)
		})
	}

}
