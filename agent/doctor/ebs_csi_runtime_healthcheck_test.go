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
package doctor

import (
	"errors"
	"testing"

	mock_csiclient "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Tests that EBS Daemon Health Check is of the right health check type
func TestEBSGetHealthcheckType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	csiClient := mock_csiclient.NewMockCSIClient(ctrl)
	hc := NewEBSCSIDaemonHealthCheck(csiClient, 0)

	assert.Equal(t, doctor.HealthcheckTypeEBSDaemon, hc.GetHealthcheckType())
}

// Tests initial health status of EBS Daemon
func TestEBSInitialHealth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	csiClient := mock_csiclient.NewMockCSIClient(ctrl)
	hc := NewEBSCSIDaemonHealthCheck(csiClient, 0)

	assert.Equal(t, doctor.HealthcheckStatusInitializing, hc.GetHealthcheckStatus())
}

// Tests RunCheck method of EBS Daemon Health Check
func TestEBSRunHealthCheck(t *testing.T) {
	tcs := []struct {
		name                     string
		setCSIClientExpectations func(csiClient *mock_csiclient.MockCSIClient)
		expectedStatus           doctor.HealthcheckStatus
	}{
		{
			name: "OK when healthcheck succeeds",
			setCSIClientExpectations: func(csiClient *mock_csiclient.MockCSIClient) {
				csiClient.EXPECT().NodeGetCapabilities(gomock.Any()).
					Return(&csi.NodeGetCapabilitiesResponse{}, nil)
			},
			expectedStatus: doctor.HealthcheckStatusOk,
		},
		{
			name: "IMPAIRED when healthcheck fails",
			setCSIClientExpectations: func(csiClient *mock_csiclient.MockCSIClient) {
				csiClient.EXPECT().NodeGetCapabilities(gomock.Any()).Return(nil, errors.New("err"))
			},
			expectedStatus: doctor.HealthcheckStatusImpaired,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			csiClient := mock_csiclient.NewMockCSIClient(ctrl)
			if tc.setCSIClientExpectations != nil {
				tc.setCSIClientExpectations(csiClient)
			}
			hc := NewEBSCSIDaemonHealthCheck(csiClient, 0)

			assert.Equal(t, tc.expectedStatus, hc.RunCheck())
		})
	}
}
