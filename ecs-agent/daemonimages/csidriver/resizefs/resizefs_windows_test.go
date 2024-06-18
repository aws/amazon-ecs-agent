//go:build windows
// +build windows

package resizefs

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/mounter"
)

func TestResize(t *testing.T) {
	testCases := []struct {
		name            string
		deviceMountPath string
		expectedResult  bool
		expectedError   bool
		prepare         func(m *mounter.MockProxyMounter)
	}{
		{
			name:            "success: normal",
			deviceMountPath: "/mnt/test",
			expectedResult:  true,
			expectedError:   false,
			prepare: func(m *mounter.MockProxyMounter) {
				m.EXPECT().ResizeVolume(gomock.Eq("/mnt/test")).Return(true, nil)
			},
		},
		{
			name:            "failure: invalid device mount path",
			deviceMountPath: "/",
			expectedResult:  false,
			expectedError:   true,
			prepare: func(m *mounter.MockProxyMounter) {
				m.EXPECT().ResizeVolume(gomock.Eq("/")).Return(false, errors.New("Could not resize volume"))
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			mockProxyMounter := mounter.NewMockProxyMounter(mockCtl)
			tc.prepare(mockProxyMounter)

			r := NewResizeFs(mockProxyMounter)
			res, err := r.Resize("", tc.deviceMountPath)

			if tc.expectedError && err == nil {
				t.Fatalf("Expected error, but got no error")
			}
			if res != tc.expectedResult {
				t.Fatalf("Expected result is: %v, but got: %v", tc.expectedResult, res)
			}
		})
	}
}
