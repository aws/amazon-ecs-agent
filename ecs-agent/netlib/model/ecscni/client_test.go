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

package ecscni

import (
	"context"
	"fmt"
	"testing"

	mock_libcni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_libcni"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAddDel(t *testing.T) {
	testCases := []struct {
		name     string
		addError bool
		delError bool
	}{
		{
			name:     "no error",
			addError: false,
			delError: false,
		},
		{
			name:     "add error",
			addError: true,
			delError: false,
		},
		{
			name:     "del error",
			addError: false,
			delError: true,
		},
		{
			name:     "both error",
			addError: true,
			delError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ctx, _ := context.WithCancel(context.TODO())

			client := NewCNIClient([]string{})
			mockLibCNIClient := mock_libcni.NewMockCNI(ctrl)
			client.(*cniClient).cni = mockLibCNIClient

			config := &TestCNIConfig{
				CNIConfig: CNIConfig{
					NetNSPath:      NetNS,
					CNISpecVersion: CNIVersion,
					CNIPluginName:  PluginName,
				},
				NetworkInterfaceName: IfName,
			}

			var addErrRet, delErrRet error
			var addRst types.Result
			if tc.addError {
				addErrRet = fmt.Errorf("add error")
			} else {
				addRst = &TestResult{
					msg: CNIVersion,
				}
			}
			if tc.delError {
				delErrRet = fmt.Errorf("del error")
			}
			mockLibCNIClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(addRst, addErrRet)
			mockLibCNIClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(delErrRet)

			rst, addErr := client.Add(ctx, config)
			delErr := client.Del(ctx, config)

			if tc.addError {
				assert.Error(t, addErr, "expecting error from add operation")
				assert.Nil(t, rst)
			} else {
				assert.NoError(t, addErr, "expecting no error from add operation")
				assert.NotNil(t, rst, "expecting result from add operation")
			}
			if tc.delError {
				assert.Error(t, delErr, "expecting error from del operation")
			} else {
				assert.NoError(t, delErr, "expecting no error from del operation")
			}
		})
	}
}
