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
	"github.com/aws/amazon-ecs-agent/ecs-agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	subnetGatewayIpv6Address = "2600:1f14:30ab:6901::/64"
	externalReason           = "external reason"
	containerArn             = "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000"
	timestamp                = "2025-04-29T16:56:17.446028948Z"
	launchType               = "EC2"
	clockErrorBound          = 1234
	utilizedMiBs             = 500
	reservedMiBs             = 600
	numProcs                 = 2
	containerStatsName       = "name"
	containerStatsId         = "id"
	networkStatsKey          = "a"
	rxBytes                  = 5
	rxBytesPerSecond         = 10
	txBytesPerSecond         = 15
	// Common Container/Task metadata JSON response templates
	containerResponseJSON = `{"DockerId":"%s","Name":"%s","DockerName":"%s","Image":"%s","ImageID":"%s",` +
		`"Ports":[{"ContainerPort":%d,"Protocol":"%s","HostPort":%d}],"Labels":{"foo":"bar"},"DesiredStatus":"%s",` +
		`"KnownStatus":"%s","Limits":{"CPU":%d,"Memory":%d},"Type":"%s","ContainerARN":"%s","Networks":[{"NetworkMode":"%s",` +
		`"IPv4Addresses":["%s"],"AttachmentIndex":0,"MACAddress":"%s","IPv4SubnetCIDRBlock":"%s","PrivateDNSName":"%s","SubnetGatewayIpv4Address":"%s","SubnetGatewayIpv6Address":"%s"}]}`
	taskResponseJSON = `{"Cluster":"%s","TaskARN":"%s","Family":"%s","Revision":"%s","DesiredStatus":"%s",` +
		`"KnownStatus":"%s","Limits":{"CPU":%d,"Memory":%d},"PullStartedAt":"%s","PullStoppedAt":"%s","ExecutionStoppedAt":"%s",` +
		`"AvailabilityZone":"%s","LaunchType":"%s","Containers":[%s],"VPCID":"%s","ClockDrift":{"ClockErrorBound":%d,` +
		`"ClockSynchronizationStatus":"%s"},"EphemeralStorageMetrics":{"Utilized":%d,"Reserved":%d},"FaultInjectionEnabled":%t}`
	containerStatsResponseJSON = `{"read":"0001-01-01T00:00:00Z","preread":"0001-01-01T00:00:00Z","pids_stats":{},"blkio_stats":` +
		`{"io_service_bytes_recursive":null,"io_serviced_recursive":null,"io_queue_recursive":null,"io_service_time_recursive":null,` +
		`"io_wait_time_recursive":null,"io_merged_recursive":null,"io_time_recursive":null,"sectors_recursive":null},"num_procs":%d,` +
		`"storage_stats":{},"cpu_stats":{"cpu_usage":{"total_usage":0,"usage_in_kernelmode":0,"usage_in_usermode":0},"throttling_data"` +
		`:{"periods":0,"throttled_periods":0,"throttled_time":0}},"precpu_stats":{"cpu_usage":{"total_usage":0,"usage_in_kernelmode":0,` +
		`"usage_in_usermode":0},"throttling_data":{"periods":0,"throttled_periods":0,"throttled_time":0}},"memory_stats":{},"name":"%s",` +
		`"id":"%s","networks":{"%s":{"rx_bytes":%d,"rx_packets":0,"rx_errors":0,"rx_dropped":0,"tx_bytes":0,"tx_packets":0,"tx_errors":0,` +
		`"tx_dropped":0}},"network_rate_stats":{"rx_bytes_per_sec":%d,"tx_bytes_per_sec":%d}}`
	taskStatsResponseJSON = `{"%s":%s}`
	// Common Container/Task metadata string response template
	responseStringMessage = "\"%s\""
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
				SubnetGatewayIPV6Address: subnetGatewayIpv6Address,
			}},
		},
	}
	now            = time.Now()
	credentialsID  = "credentialsID"
	containerStats = state.StatsResponse{
		StatsJSON: &types.StatsJSON{
			Stats:    types.Stats{NumProcs: numProcs},
			Name:     containerStatsName,
			ID:       containerStatsId,
			Networks: map[string]types.NetworkStats{networkStatsKey: {RxBytes: rxBytes}},
		},
		Network_rate_stats: &stats.NetworkStatsPerSec{
			RxBytesPerSecond: rxBytesPerSecond,
			TxBytesPerSecond: txBytesPerSecond,
		},
	}
	taskStats = map[string]*state.StatsResponse{
		containerID: &containerStats,
	}
	happyContainerResponseJSON = fmt.Sprintf(containerResponseJSON,
		containerID,
		containerName,
		containerName,
		imageName,
		imageID,
		containerPort,
		containerPortProtocol,
		containerPort,
		statusRunning,
		statusRunning,
		cpu,
		memory,
		containerType,
		containerArn,
		utils.NetworkModeAWSVPC,
		eniIPv4Address,
		macAddress,
		iPv4SubnetCIDRBlock,
		privateDNSName,
		subnetGatewayIpv4Address,
		subnetGatewayIpv6Address,
	)
	happyContainerStatsResponseJSON = fmt.Sprintf(containerStatsResponseJSON,
		numProcs,
		containerStatsName,
		containerStatsId,
		networkStatsKey,
		rxBytes,
		rxBytesPerSecond,
		txBytesPerSecond,
	)
)

// taskResponse returns a standard agent task response
func taskResponse() *state.TaskResponse {
	return &state.TaskResponse{
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
		ClockDrift: &state.ClockDrift{
			ClockErrorBound:            1234,
			ClockSynchronizationStatus: state.ClockStatusSynchronized,
		},
		EphemeralStorageMetrics: &state.EphemeralStorageMetrics{
			UtilizedMiBs: 500,
			ReservedMiBs: 600,
		},
		CredentialsID: credentialsID,
		TaskNetworkConfig: &state.TaskNetworkConfig{
			NetworkMode: utils.NetworkModeAWSVPC,
			NetworkNamespaces: []*state.NetworkNamespace{
				&state.NetworkNamespace{
					Path: "/var/run/netns/8059dc9193014dfeaab22d7a9997afad-064c910879c7",
					NetworkInterfaces: []*state.NetworkInterface{
						&state.NetworkInterface{
							DeviceName: "eth1",
						},
					},
				},
			},
		},
	}
}

// taskResponseWithFaultInjectionEnabled returns a standard agent task response with FaultInjection enabled
func taskResponseWithFaultInjectionEnabled() *state.TaskResponse {
	taskResponse := taskResponse()
	taskResponse.FaultInjectionEnabled = true
	return taskResponse
}

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
			expectedResponseJSON: happyContainerResponseJSON,
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
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "V4 container metadata handler: "+externalReason),
		})
	})
	t.Run("failed to get metadata", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := state.NewErrorMetadataFetchFailure(externalReason)
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err)
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(state.ContainerResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: externalReason,
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, externalReason),
		})
	})
	t.Run("unknown error returned by AgentState", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := errors.New("unknown")
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err)
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetContainerMetadata(endpointContainerID).
			Return(state.ContainerResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 "/v4/" + endpointContainerID,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: fmt.Sprintf("failed to get container metadata"),
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "failed to get container metadata"),
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
		metadata := taskResponse()
		expectedTaskResponse := taskResponse()
		expectedTaskResponse.CredentialsID = ""      // credentials ID not expected
		expectedTaskResponse.TaskNetworkConfig = nil // TaskNetworkConfig is not expected and would be used internally.
		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(*metadata, nil)
		testTMDSRequest(t, handler, TMDSTestCase[state.TaskResponse]{
			path:                 path,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: *expectedTaskResponse,
			expectedResponseJSON: fmt.Sprintf(taskResponseJSON,
				clusterName,
				taskARN,
				family,
				version,
				statusRunning,
				statusRunning,
				cpu,
				memory,
				now.UTC().Format(time.RFC3339Nano),
				now.UTC().Format(time.RFC3339Nano),
				now.UTC().Format(time.RFC3339Nano),
				availabilityzone,
				launchType,
				happyContainerResponseJSON,
				vpcID,
				clockErrorBound,
				state.ClockStatusSynchronized,
				utilizedMiBs,
				reservedMiBs,
				false,
			),
		})
	})

	t.Run("happy case with FaultInjection enabled", func(t *testing.T) {
		metadata := taskResponseWithFaultInjectionEnabled()
		expectedTaskResponse := taskResponseWithFaultInjectionEnabled()
		expectedTaskResponse.CredentialsID = ""      // credentials ID not expected
		expectedTaskResponse.TaskNetworkConfig = nil // TaskNetworkConfig is not expected and would be used internally

		handler, _, agentState, _ := setup(t)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(*metadata, nil)
		testTMDSRequest(t, handler, TMDSTestCase[state.TaskResponse]{
			path:                 path,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: *expectedTaskResponse,
			expectedResponseJSON: fmt.Sprintf(taskResponseJSON,
				clusterName,
				taskARN,
				family,
				version,
				statusRunning,
				statusRunning,
				cpu,
				memory,
				now.UTC().Format(time.RFC3339Nano),
				now.UTC().Format(time.RFC3339Nano),
				now.UTC().Format(time.RFC3339Nano),
				availabilityzone,
				launchType,
				happyContainerResponseJSON,
				vpcID,
				clockErrorBound,
				state.ClockStatusSynchronized,
				utilizedMiBs,
				reservedMiBs,
				true,
			),
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
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "V4 task metadata handler: task lookup failed"),
		})
	})
	t.Run("metadata fetch failure", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := state.NewErrorMetadataFetchFailure(externalReason)
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err)
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(externalReason))

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: externalReason,
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, externalReason),
		})
	})
	t.Run("unknown error returned by AgentState", func(t *testing.T) {
		handler, ctrl, agentState, metricsFactory := setup(t)

		err := errors.New("unknown")
		entry := mock_metrics.NewMockEntry(ctrl)

		entry.EXPECT().Done(err)
		metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)
		agentState.EXPECT().
			GetTaskMetadata(endpointContainerID).
			Return(state.TaskResponse{}, err)

		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: "failed to get task metadata",
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "failed to get task metadata"),
		})
	})
}

func TestContainerStatsPath(t *testing.T) {
	assert.Equal(t, "/v4/{endpointContainerIDMuxName:[^/]*}/stats", ContainerStatsPath())
}

func TestTaskStatsPath(t *testing.T) {
	assert.Equal(t, "/v4/{endpointContainerIDMuxName:[^/]*}/task/stats", TaskStatsPath())
}

func TestContainerStats(t *testing.T) {
	// path for the stats endpoint
	path := fmt.Sprintf("/v4/%s/stats", endpointContainerID)

	// helper function to setup mocks and a handler with container stats endpoint
	setup := func() (
		*mock_state.MockAgentState, *gomock.Controller, *mock_metrics.MockEntryFactory, http.Handler,
	) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		agentState := mock_state.NewMockAgentState(ctrl)
		metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		router := mux.NewRouter()
		router.HandleFunc(
			ContainerStatsPath(),
			ContainerStatsHandler(agentState, metricsFactory))

		return agentState, ctrl, metricsFactory, router
	}

	// Test cases start here
	t.Run("stats lookup failure", func(t *testing.T) {
		agentState, _, _, handler := setup()
		agentState.EXPECT().
			GetContainerStats(endpointContainerID).
			Return(state.StatsResponse{}, state.NewErrorStatsLookupFailure(externalReason))
		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusNotFound,
			expectedResponseBody: "V4 container stats handler: " + externalReason,
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "V4 container stats handler: "+externalReason),
		})
	})

	internalServerErrorCases := []struct {
		err          error
		responseBody string
	}{
		{
			err:          state.NewErrorStatsFetchFailure(externalReason, errors.New("cause")),
			responseBody: externalReason,
		},
		{
			err:          errors.New("unknown error"),
			responseBody: "failed to get stats",
		},
	}
	for _, tc := range internalServerErrorCases {
		t.Run("stats fetch failure", func(t *testing.T) {
			agentState, ctrl, metricsFactory, handler := setup()

			// Expectations
			agentState.EXPECT().
				GetContainerStats(endpointContainerID).
				Return(state.StatsResponse{}, tc.err)

			// Expect InternalServerError metric to be published with the error.
			entry := mock_metrics.NewMockEntry(ctrl)
			entry.EXPECT().Done(tc.err)
			metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)

			// Make test request
			testTMDSRequest(t, handler, TMDSTestCase[string]{
				path:                 path,
				expectedStatusCode:   http.StatusInternalServerError,
				expectedResponseBody: tc.responseBody,
				expectedResponseJSON: fmt.Sprintf(responseStringMessage, tc.responseBody),
			})
		})
	}

	t.Run("happy case", func(t *testing.T) {
		agentState, _, _, handler := setup()
		agentState.EXPECT().
			GetContainerStats(endpointContainerID).
			Return(containerStats, nil)
		testTMDSRequest(t, handler, TMDSTestCase[state.StatsResponse]{
			path:                 path,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: containerStats,
			expectedResponseJSON: happyContainerStatsResponseJSON,
		})
	})
}

func TestTaskStats(t *testing.T) {
	// path for the stats endpoint
	path := fmt.Sprintf("/v4/%s/task/stats", endpointContainerID)

	// helper function to setup mocks and a handler with container stats endpoint
	setup := func() (
		*mock_state.MockAgentState, *gomock.Controller, *mock_metrics.MockEntryFactory, http.Handler,
	) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		agentState := mock_state.NewMockAgentState(ctrl)
		metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		router := mux.NewRouter()
		router.HandleFunc(
			TaskStatsPath(),
			TaskStatsHandler(agentState, metricsFactory))

		return agentState, ctrl, metricsFactory, router
	}

	// Test cases start here
	t.Run("stats lookup failure", func(t *testing.T) {
		agentState, _, _, handler := setup()
		agentState.EXPECT().
			GetTaskStats(endpointContainerID).
			Return(nil, state.NewErrorStatsLookupFailure(externalReason))
		testTMDSRequest(t, handler, TMDSTestCase[string]{
			path:                 path,
			expectedStatusCode:   http.StatusNotFound,
			expectedResponseBody: "V4 task stats handler: " + externalReason,
			expectedResponseJSON: fmt.Sprintf(responseStringMessage, "V4 task stats handler: "+externalReason),
		})
	})

	internalServerErrorCases := []struct {
		err          error
		responseBody string
	}{
		{
			err:          state.NewErrorStatsFetchFailure(externalReason, errors.New("cause")),
			responseBody: externalReason,
		},
		{
			err:          errors.New("unknown error"),
			responseBody: "failed to get stats",
		},
	}
	for _, tc := range internalServerErrorCases {
		t.Run("stats fetch failure", func(t *testing.T) {
			// setup
			agentState, ctrl, metricsFactory, handler := setup()

			// expect GetContainerStats to be called that should return an error
			agentState.EXPECT().
				GetTaskStats(endpointContainerID).
				Return(nil, tc.err)

			// expect InternalServerError metric to be published with the error.
			entry := mock_metrics.NewMockEntry(ctrl)
			entry.EXPECT().Done(tc.err)
			metricsFactory.EXPECT().New(metrics.InternalServerErrorMetricName).Return(entry)

			// Go
			testTMDSRequest(t, handler, TMDSTestCase[string]{
				path:                 path,
				expectedStatusCode:   http.StatusInternalServerError,
				expectedResponseBody: tc.responseBody,
				expectedResponseJSON: fmt.Sprintf(responseStringMessage, tc.responseBody),
			})
		})
	}

	t.Run("happy case", func(t *testing.T) {
		agentState, _, _, handler := setup()
		agentState.EXPECT().
			GetTaskStats(endpointContainerID).
			Return(taskStats, nil)
		testTMDSRequest(t, handler, TMDSTestCase[map[string]*state.StatsResponse]{
			path:                 path,
			expectedStatusCode:   http.StatusOK,
			expectedResponseBody: taskStats,
			expectedResponseJSON: fmt.Sprintf(taskStatsResponseJSON, containerID, happyContainerStatsResponseJSON),
		})
	})
}

type TMDSResponse interface {
	string |
		state.ContainerResponse |
		state.TaskResponse |
		state.StatsResponse |
		map[string]*state.StatsResponse
}

type TMDSTestCase[R TMDSResponse] struct {
	path                 string
	expectedStatusCode   int
	expectedResponseBody R
	expectedResponseJSON string
}

func testTMDSRequest[R TMDSResponse](t *testing.T, handler http.Handler, tc TMDSTestCase[R]) {
	// Create the request
	req, err := http.NewRequest("GET", tc.path, nil)
	require.NoError(t, err)

	// Send the request and record the response
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Assert that the response JSON string is as expected.
	assert.Equal(t, tc.expectedResponseJSON, recorder.Body.String())

	// Parse the response body
	var actualResponseBody R
	err = json.Unmarshal(recorder.Body.Bytes(), &actualResponseBody)
	require.NoError(t, err)

	// Assert status code and body
	assert.Equal(t, tc.expectedStatusCode, recorder.Code)
	assert.Equal(t, tc.expectedResponseBody, actualResponseBody)
}
