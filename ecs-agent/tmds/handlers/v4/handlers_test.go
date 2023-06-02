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
package v4

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
)

const (
	clusterName              = "default"
	taskARN                  = "taskARN"
	family                   = "family"
	endpointContainerID      = "endpointContainerID"
	vpcID                    = "vpcID"
	availabilityzone         = "availabilityZone"
	containerID              = "cid"
	containerName            = "sleepy"
	imageName                = "busybox"
	imageID                  = "bUsYbOx"
	cpu                      = 1024
	memory                   = 512
	statusRunning            = "RUNNING"
	containerType            = "NORMAL"
	containerPort            = 80
	containerPortProtocol    = "tcp"
	eniIPv4Address           = "10.0.0.2"
	iPv4SubnetCIDRBlock      = "172.31.32.0/20"
	macAddress               = "06:96:9a:ce:a6:ce"
	privateDNSName           = "ip-172-31-47-69.us-west-2.compute.internal"
	subnetGatewayIpv4Address = "172.31.32.1/20"
	externalReason           = "external reason"
)

var (
	attachmentIndex = 0
	labels          = map[string]string{
		"foo": "bar",
	}
	containerResponse = state.ContainerResponse{
		ContainerResponse: &v2.ContainerResponse{
			ID:            containerID,
			Name:          containerName,
			DockerName:    containerName,
			Image:         imageName,
			ImageID:       imageID,
			DesiredStatus: statusRunning,
			KnownStatus:   statusRunning,
			ContainerARN:  "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
			Limits: v2.LimitsResponse{
				CPU:    aws.Float64(cpu),
				Memory: aws.Int64(memory),
			},
			Type:   containerType,
			Labels: labels,
			Ports: []response.PortResponse{
				{
					ContainerPort: containerPort,
					Protocol:      containerPortProtocol,
					HostPort:      containerPort,
				},
			},
		},
		Networks: []state.Network{{
			Network: response.Network{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: []string{eniIPv4Address},
			},
			NetworkInterfaceProperties: state.NetworkInterfaceProperties{
				AttachmentIndex:          &attachmentIndex,
				IPV4SubnetCIDRBlock:      iPv4SubnetCIDRBlock,
				MACAddress:               macAddress,
				PrivateDNSName:           privateDNSName,
				SubnetGatewayIPV4Address: subnetGatewayIpv4Address,
			}},
		},
	}
	now          = time.Now()
	taskResponse = state.TaskResponse{
		TaskResponse: &v2.TaskResponse{
			Cluster:       clusterName,
			TaskARN:       taskARN,
			Family:        family,
			Revision:      version,
			DesiredStatus: statusRunning,
			KnownStatus:   statusRunning,
			Limits: &v2.LimitsResponse{
				CPU:    aws.Float64(cpu),
				Memory: aws.Int64(memory),
			},
			PullStartedAt:      aws.Time(now.UTC()),
			PullStoppedAt:      aws.Time(now.UTC()),
			ExecutionStoppedAt: aws.Time(now.UTC()),
			AvailabilityZone:   availabilityzone,
			LaunchType:         "EC2",
		},
		Containers: []state.ContainerResponse{containerResponse},
		VPCID:      vpcID,
		ClockDrift: state.ClockDrift{
			ClockErrorBound:            1234,
			ClockSynchronizationStatus: state.ClockStatusSynchronized,
		},
		EphemeralStorageMetrics: state.EphemeralStorageMetrics{
			UtilizedMiBs: 500,
			ReservedMiBs: 600,
		},
	}
)

func TestContainerMetadata(t *testing.T) {
	var setup = func(t *testing.T) (*mux.Router, *gomock.Controller, *mock_state.MockAgentState,
		*mock_metrics.MockEntryFactory,
	) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		agentState := mock_state.NewMockAgentState(ctrl)
		metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		router := mux.NewRouter()
		router.HandleFunc(
			ContainerMetadataPath(),
			ContainerMetadataHandler(agentState, metricsFactory))

		return router, ctrl, agentState, metricsFactory
	}

	t.Run("happy case", func(t *testing.T) {
		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(containerResponse, nil)
		testTMDSRequest(t, handler, TMDSTestCase[state.ContainerResponse]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: containerResponse,
		})
	})
	t.Run("container lookup failed", func(t *testing.T) {
		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(state.ContainerResponse{}, state.NewErrorLookupFailure(externalReason))
		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusNotFound,
			expectedResponseBody: "V4 container metadata handler: " + externalReason,
		})
	})
	t.Run("failed to get metadata", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := state.NewErrorMetadataFetchFailure(externalReason)
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err).Return(func() {})
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(state.ContainerResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: externalReason,
		})
	})
	t.Run("unknown error returned by AgentState", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := errors.New("unknown")
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err).Return(func() {})
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(state.ContainerResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: fmt.Sprintf("failed to get container metadata"),
		})
	})
}

func TestTaskMetadata(t *testing.T) {
	path := fmt.Sprintf("/v4/%s/task", endpointContainerID)

	var setup = func(t *testing.T) (*mux.Router, *gomock.Controller, *mock_state.MockAgentState,
		*mock_metrics.MockEntryFactory,
	) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		agentState := mock_state.NewMockAgentState(ctrl)
		metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		router := mux.NewRouter()
		router.HandleFunc(
			TaskMetadataPath(),
			TaskMetadataHandler(agentState, metricsFactory))

		return router, ctrl, agentState, metricsFactory
	}

	t.Run("happy case", func(t *testing.T) {
		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(taskResponse, nil)
		testTMDSRequest(t, handler, TMDSTestCase[state.TaskResponse]{
			path:                 path,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: taskResponse,
		})
	})
	t.Run("task lookup failure", func(t *testing.T) {
		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed"))
		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusNotFound,
			expectedResponseBody: "V4 task metadata handler: task lookup failed",
		})
	})
	t.Run("metadata fetch failure", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := state.NewErrorMetadataFetchFailure(externalReason)
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err).Return(func() {})
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(externalReason))

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: externalReason,
		})
	})
	t.Run("unknown error returned by AgentState", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := errors.New("unknown")
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err).Return(func() {})
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(state.TaskResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: "failed to get task metadata",
		})
	})
}

type TMDSResponse interface {
	string | state.ContainerResponse | state.TaskResponse
}

type TMDSTestCase[R TMDSResponse] struct {
	path                 string
	expectedStatusCode   int
	expectedResponseBody R
}

func testTMDSRequest[R TMDSResponse](t *testing.T, handler http.Handler, tc TMDSTestCase[R]) {
	// Create the request
	req, err := http.NewRequest("GET", tc.path, nil)
	require.NoError(t, err)

	// Send the request and record the response
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Parse the response body
	var actualResponseBody R
	err = json.Unmarshal(recorder.Body.Bytes(), &actualResponseBody)
	require.NoError(t, err)

	// Assert status code and body
	assert.Equal(t, tc.expectedStatusCode, recorder.Code)
	assert.Equal(t, tc.expectedResponseBody, actualResponseBody)
}
