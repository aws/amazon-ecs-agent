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

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	taskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
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

	getter := mock_engine.NewMockEBSDaemonTaskGetter(ctrl)
	csiClient := mock_csiclient.NewMockCSIClient(ctrl)
	hc := NewEBSCSIDaemonHealthCheck(getter, csiClient, 0)

	assert.Equal(t, doctor.HealthcheckTypeEBSDaemon, hc.GetHealthcheckType())
}

// Tests initial health status of EBS Daemon
func TestEBSInitialHealth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	getter := mock_engine.NewMockEBSDaemonTaskGetter(ctrl)
	csiClient := mock_csiclient.NewMockCSIClient(ctrl)
	hc := NewEBSCSIDaemonHealthCheck(getter, csiClient, 0)

	assert.Equal(t, doctor.HealthcheckStatusInitializing, hc.GetHealthcheckStatus())
}

// Tests RunCheck method of EBS Daemon Health Check
func TestEBSRunHealthCheck(t *testing.T) {
	tcs := []struct {
		name                     string
		setGetterExpectations    func(getter *mock_engine.MockEBSDaemonTaskGetter)
		setCSIClientExpectations func(csiClient *mock_csiclient.MockCSIClient)
		expectedStatus           doctor.HealthcheckStatus
	}{
		{
			name: "Initializing when daemon task is not initialized",
			setGetterExpectations: func(getter *mock_engine.MockEBSDaemonTaskGetter) {
				getter.EXPECT().GetEbsDaemonTask().Return(nil)
			},
			expectedStatus: doctor.HealthcheckStatusInitializing,
		},
		{
			name: "Initializing when daemon task is CREATED",
			setGetterExpectations: func(getter *mock_engine.MockEBSDaemonTaskGetter) {
				task := &task.Task{KnownStatusUnsafe: taskstatus.TaskCreated}
				getter.EXPECT().GetEbsDaemonTask().Return(task)
			},
			expectedStatus: doctor.HealthcheckStatusInitializing,
		},
		{
			name: "IMPAIRED when daemon task is STOPPED",
			setGetterExpectations: func(getter *mock_engine.MockEBSDaemonTaskGetter) {
				task := &task.Task{KnownStatusUnsafe: taskstatus.TaskStopped}
				getter.EXPECT().GetEbsDaemonTask().Return(task)
			},
			expectedStatus: doctor.HealthcheckStatusImpaired,
		},
		{
			name: "OK when daemon task is RUNNING and healthcheck succeeds",
			setGetterExpectations: func(getter *mock_engine.MockEBSDaemonTaskGetter) {
				task := &task.Task{KnownStatusUnsafe: taskstatus.TaskRunning}
				getter.EXPECT().GetEbsDaemonTask().Return(task)
			},
			setCSIClientExpectations: func(csiClient *mock_csiclient.MockCSIClient) {
				csiClient.EXPECT().NodeGetCapabilities(gomock.Any()).
					Return(&csi.NodeGetCapabilitiesResponse{}, nil)
			},
			expectedStatus: doctor.HealthcheckStatusOk,
		},
		{
			name: "IMPAIRED when daemon task is RUNNING and healthcheck fails",
			setGetterExpectations: func(getter *mock_engine.MockEBSDaemonTaskGetter) {
				task := &task.Task{KnownStatusUnsafe: taskstatus.TaskRunning}
				getter.EXPECT().GetEbsDaemonTask().Return(task)
			},
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

			getter := mock_engine.NewMockEBSDaemonTaskGetter(ctrl)
			csiClient := mock_csiclient.NewMockCSIClient(ctrl)
			if tc.setGetterExpectations != nil {
				tc.setGetterExpectations(getter)
			}
			if tc.setCSIClientExpectations != nil {
				tc.setCSIClientExpectations(csiClient)
			}
			hc := NewEBSCSIDaemonHealthCheck(getter, csiClient, 0)

			assert.Equal(t, tc.expectedStatus, hc.RunCheck())
		})
	}
}
