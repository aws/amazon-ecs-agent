//go:build linux && sudo
// +build linux,sudo

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

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	agentV4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	faulttype "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validTaskArnPrefix       = "arn:aws:ecs:region:account-id:task/"
	testTaskId               = "fault-injection-task"
	testContainerName        = "fault-injection-container"
	testImage                = "public.ecr.aws/amazonlinux/amazonlinux:2"
	endpointId               = "endpoint"
	taskARN                  = "t1"
	containerID              = "cid"
	containerName            = "sleepy"
	imageName                = "busybox"
	imageID                  = "bUsYbOx"
	cpu                      = 1024
	memory                   = 512
	containerPort            = 80
	associationType          = "elastic-inference"
	associationName          = "dev1"
	associationEncoding      = "base64"
	associationValue         = "val"
	family                   = "sleep"
	version                  = "1"
	eniIPv4Address           = "10.0.0.2"
	macAddress               = "06:96:9a:ce:a6:ce"
	privateDNSName           = "ip-172-31-47-69.us-west-2.compute.internal"
	subnetGatewayIpv4Address = "172.31.32.1/20"
	taskCredentialsID        = "taskCredentialsId"
	defaultIfname            = "eth0"
	clusterName              = "default"
	availabilityzone         = "us-west-2b"
	vpcID                    = "test-vpc-id"
	containerInstanceArn     = "containerInstanceArn-test"
	startEndpoint            = "/api/%s/fault/v1/%s/start"
	stopEndpoint             = "/api/%s/fault/v1/%s/stop"
	statusEndpoint           = "/api/%s/fault/v1/%s/status"
	defaultNamespace         = "host"
	requestMaxRetries        = 3
	requestTimeoutInSeconds  = 5 * time.Second

	port               = 4321
	protocol           = "tcp"
	trafficType        = "ingress"
	delayMilliseconds  = 100
	jitterMilliseconds = 200
	lossPercent        = 6

	serverPort = 3333
)

var (
	now = time.Now()

	ipSources         = []string{"1.1.1.1", "2.2.2.2"}
	ipSourcesToFilter = []string{"8.8.8.8"}

	happyBlackHolePortReqBody = map[string]interface{}{
		"Port":        port,
		"Protocol":    protocol,
		"TrafficType": trafficType,
	}

	happyNetworkLatencyReqBody = map[string]interface{}{
		"DelayMilliseconds":  delayMilliseconds,
		"JitterMilliseconds": jitterMilliseconds,
		"Sources":            ipSources,
		"SourcesToFilter":    ipSourcesToFilter,
	}

	happyNetworkPacketLossReqBody = map[string]interface{}{
		"LossPercent":     lossPercent,
		"Sources":         ipSources,
		"SourcesToFilter": ipSourcesToFilter,
	}

	container = &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		ContainerArn:        "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
		KnownPortBindingsUnsafe: []apicontainer.PortBinding{
			{
				ContainerPort: containerPort,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	dockerContainer = &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}

	containerNameToDockerContainer = map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}

	association = apitask.Association{
		Containers: []string{containerName},
		Content: apitask.EncodedString{
			Encoding: associationEncoding,
			Value:    associationValue,
		},
		Name: associationName,
		Type: associationType,
	}

	agentStateExpectations = func(state *mock_dockerstate.MockTaskEngineState) {
		task := standardFaultInjectionTask()
		state.EXPECT().TaskARNByV3EndpointID(endpointId).Return(taskARN, true).AnyTimes()
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes()
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true).AnyTimes()
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes()
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true).AnyTimes()
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true).AnyTimes()
	}
)

func standardFaultInjectionTask() *apitask.Task {
	task := apitask.Task{
		Arn:                 taskARN,
		Associations:        []apitask.Association{association},
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		NetworkMode:         apitask.HostNetworkMode,
		ENIs: []*ni.NetworkInterface{
			{
				IPV4Addresses: []*ni.IPV4Address{
					{
						Address: eniIPv4Address,
					},
				},
				MacAddress:               macAddress,
				PrivateDNSName:           privateDNSName,
				SubnetGatewayIPV4Address: subnetGatewayIpv4Address,
			},
		},
		CPU:                      cpu,
		Memory:                   memory,
		PullStartedAtUnsafe:      now,
		PullStoppedAtUnsafe:      now,
		ExecutionStoppedAtUnsafe: now,
		LaunchType:               "EC2",
		NetworkNamespace:         defaultNamespace,
		EnableFaultInjection:     true,
	}
	task.SetCredentialsID(taskCredentialsID)
	return &task
}

// This function starts the server and listens on a specified port
func startServer(t *testing.T) (*http.Server, int) {
	router := mux.NewRouter()

	// Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	agentStateExpectations(state)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)

	agentState := agentV4.NewTMDSAgentState(state, statsEngine, ecsClient, clusterName, availabilityzone, vpcID, containerInstanceArn)
	metricsFactory := metrics.NewNopEntryFactory()
	execWrapper := execwrapper.NewExec()

	registerFaultHandlers(router, agentState, metricsFactory, execWrapper)

	server := &http.Server{
		Addr:    ":0", // Lets the system allocate an available port
		Handler: router,
	}

	listener, err := net.Listen("tcp", server.Addr)
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port
	t.Logf("Server started on port: %d", port)

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("ListenAndServe(): %s\n", err)
		}
	}()
	return server, port
}

// This function shuts down the server after the test
func shutdownServer(t *testing.T, server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Println("Server shutdown fail")
		t.Logf("Server Shutdown Failed:%+v", err)
	} else {
		fmt.Println("Server shutdown")
		t.Logf("Server Exited Properly")
	}

}

// Utility function to generate a URL with a dynamic port
func getURL(port int, path string) string {
	return "http://localhost:" + fmt.Sprintf("%d", port) + path
}

func cleanupBlackHolePortFault(t *testing.T, serverPort int, body interface{}) int {
	client := &http.Client{}

	bodyBytes, _ := json.Marshal(body)
	bodyReader := bytes.NewReader(bodyBytes)
	req, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, faulttype.BlackHolePortFaultType)), bodyReader)
	res, err := client.Do(req)
	if err != nil {
		t.Logf("Error occurred while cleaning up network black hole port fault injection: %v\n", err)
	}
	return res.StatusCode
}

func cleanupLatencyAndPacketLossFaults(t *testing.T, serverPort int) {
	client := &http.Client{}
	req1, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, "network-latency")), nil)

	_, err = client.Do(req1)
	if err != nil {
		t.Logf("Error occurred while cleaning up network latency fault injection: %v\n", err)
	}

	req2, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, "network-packet-loss")), nil)
	_, err = client.Do(req2)
	if err != nil {
		t.Logf("Error occurred while cleaning up network packet loss fault injection: %v\n", err)
	}
}

func TestParallelNetworkFaults(t *testing.T) {
	tcs := []struct {
		name          string
		faultType1    string
		faultType2    string
		body1         interface{}
		body2         interface{}
		responseCode1 int
		responseCode2 int
	}{
		{
			name:          "network latency same type",
			faultType1:    faulttype.LatencyFaultType,
			faultType2:    faulttype.LatencyFaultType,
			body1:         happyNetworkLatencyReqBody,
			body2:         happyNetworkLatencyReqBody,
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network packet loss same type",
			faultType1:    faulttype.PacketLossFaultType,
			faultType2:    faulttype.PacketLossFaultType,
			body1:         happyNetworkPacketLossReqBody,
			body2:         happyNetworkPacketLossReqBody,
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network latency and packet loss different type",
			faultType1:    faulttype.LatencyFaultType,
			faultType2:    faulttype.PacketLossFaultType,
			body1:         happyNetworkLatencyReqBody,
			body2:         happyNetworkPacketLossReqBody,
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network black hole port same type",
			faultType1:    faulttype.BlackHolePortFaultType,
			faultType2:    faulttype.BlackHolePortFaultType,
			body1:         happyBlackHolePortReqBody,
			body2:         happyBlackHolePortReqBody,
			responseCode1: http.StatusOK,
			responseCode2: http.StatusOK,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Starting a server which will include the network fault handlers
			server, port := startServer(t)

			client := &http.Client{}

			// Making request 1
			bodyBytes, _ := json.Marshal(tc.body1)
			bodyReader := bytes.NewReader(bodyBytes)
			req1, err := http.NewRequest(http.MethodPost, getURL(port, fmt.Sprintf(startEndpoint, endpointId, tc.faultType1)), bodyReader)
			require.NoError(t, err)

			res1, err := client.Do(req1)
			require.NoError(t, err)

			// Making request 2
			var resp2Code int
			// Adding a retry mechanism since there's a rate limit of only one request per 5 seconds for each network fault endpoints
			for i := 0; i < requestMaxRetries; i++ {
				bodyBytes, _ = json.Marshal(tc.body2)
				bodyReader = bytes.NewReader(bodyBytes)
				req2, err := http.NewRequest(http.MethodPost, getURL(port, fmt.Sprintf(startEndpoint, endpointId, tc.faultType2)), bodyReader)
				require.NoError(t, err)
				res2, err := client.Do(req2)
				require.NoError(t, err)
				resp2Code = res2.StatusCode

				if res2.StatusCode != http.StatusTooManyRequests {
					break
				}
				t.Log("Retrying request...")
				time.Sleep(requestTimeoutInSeconds)
			}

			if tc.faultType1 == faulttype.BlackHolePortFaultType {
				cleanupBlackHolePortFault(t, port, tc.body1)
			}

			if tc.faultType2 == faulttype.BlackHolePortFaultType {
				for i := 0; i < requestMaxRetries; i++ {
					code := cleanupBlackHolePortFault(t, port, tc.body2)
					if code != http.StatusTooManyRequests {
						break
					}
					time.Sleep(requestTimeoutInSeconds)
				}
			}
			cleanupLatencyAndPacketLossFaults(t, port)

			shutdownServer(t, server)
			assert.Equal(t, tc.responseCode1, res1.StatusCode)
			assert.Equal(t, tc.responseCode2, resp2Code)
		})
	}
}
