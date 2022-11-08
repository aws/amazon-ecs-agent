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

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	task_protection_v1 "github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/taskprotection/v1/handlers"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	v4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_audit "github.com/aws/amazon-ecs-agent/agent/logger/audit/mocks"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	agentutils "github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	clusterName                = "default"
	remoteIP                   = "169.254.170.3"
	remotePort                 = "32146"
	taskARN                    = "t1"
	family                     = "sleep"
	version                    = "1"
	containerID                = "cid"
	containerName              = "sleepy"
	pulledContainerName        = "pulled"
	imageName                  = "busybox"
	imageID                    = "bUsYbOx"
	cpu                        = 1024
	memory                     = 512
	statusRunning              = "RUNNING"
	statusPulled               = "PULLED"
	statusNone                 = "NONE"
	containerType              = "NORMAL"
	containerPort              = 80
	containerPortProtocol      = "tcp"
	eniIPv4Address             = "10.0.0.2"
	roleArn                    = "r1"
	accessKeyID                = "ak"
	secretAccessKey            = "sk"
	credentialsID              = "credentialsId"
	v2BaseStatsPath            = "/v2/stats"
	v2BaseMetadataPath         = "/v2/metadata"
	v2BaseMetadataWithTagsPath = "/v2/metadataWithTags"
	v3BasePath                 = "/v3/"
	v4BasePath                 = "/v4/"
	v3EndpointID               = "v3eid"
	availabilityzone           = "us-west-2b"
	vpcID                      = "test-vpc-id"
	containerInstanceArn       = "containerInstanceArn-test"
	associationType            = "elastic-inference"
	associationName            = "dev1"
	associationEncoding        = "base64"
	associationValue           = "val"
	bridgeMode                 = "bridge"
	bridgeIPAddr               = "17.0.0.3"
	attachmentIndex            = 0
	iPv4SubnetCIDRBlock        = "172.31.32.0/20"
	macAddress                 = "06:96:9a:ce:a6:ce"
	privateDNSName             = "ip-172-31-47-69.us-west-2.compute.internal"
	subnetGatewayIpv4Address   = "172.31.32.1/20"
	region                     = "us-west-2"
	endpoint                   = "ecsEndpoint"
	acceptInsecureCert         = true
)

var (
	now         = time.Now()
	association = apitask.Association{
		Containers: []string{containerName},
		Content: apitask.EncodedString{
			Encoding: associationEncoding,
			Value:    associationValue,
		},
		Name: associationName,
		Type: associationType,
	}
	pulledAssociation = apitask.Association{
		Containers: []string{containerName, pulledContainerName},
		Content: apitask.EncodedString{
			Encoding: associationEncoding,
			Value:    associationValue,
		},
		Name: associationName,
		Type: associationType,
	}
	task = &apitask.Task{
		Arn:                 taskARN,
		Associations:        []apitask.Association{association},
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		NetworkMode:         apitask.AWSVPCNetworkMode,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
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
	}
	pulledTask = &apitask.Task{
		Arn:                 taskARN,
		Associations:        []apitask.Association{pulledAssociation},
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskStatusNone,
		NetworkMode:         apitask.AWSVPCNetworkMode,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
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
				ContainerPort: aws.Uint16(containerPort),
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}
	pulledContainer = &apicontainer.Container{
		Name:                pulledContainerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerPulled,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		ContainerArn:        "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
	}
	dockerContainer = &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}
	pulledDockerContainer = &apicontainer.DockerContainer{
		Container: pulledContainer,
	}
	containerNameToDockerContainer = map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}
	pulledContainerNameToDockerContainer = map[string]*apicontainer.DockerContainer{
		taskARN: pulledDockerContainer,
	}
	labels = map[string]string{
		"foo": "bar",
	}
	expectedContainerResponse = v2.ContainerResponse{
		ID:            containerID,
		Name:          containerName,
		DockerName:    containerName,
		Image:         imageName,
		ImageID:       imageID,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Limits: v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		Type:   containerType,
		Labels: labels,
		Ports: []v1.PortResponse{
			{
				ContainerPort: containerPort,
				Protocol:      containerPortProtocol,
				HostPort:      containerPort,
			},
		},
		Networks: []containermetadata.Network{
			{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: []string{eniIPv4Address},
			},
		},
	}
	expectedPulledContainerResponse = v2.ContainerResponse{
		Name:          pulledContainerName,
		Image:         imageName,
		ImageID:       imageID,
		DesiredStatus: statusRunning,
		KnownStatus:   statusPulled,
		Limits: v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		Type: containerType,
	}
	expectedTaskResponse = v2.TaskResponse{
		Cluster:       clusterName,
		TaskARN:       taskARN,
		Family:        family,
		Revision:      version,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Containers:    []v2.ContainerResponse{expectedContainerResponse},
		Limits: &v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		PullStartedAt:      aws.Time(now.UTC()),
		PullStoppedAt:      aws.Time(now.UTC()),
		ExecutionStoppedAt: aws.Time(now.UTC()),
		AvailabilityZone:   availabilityzone,
	}
	expectedAssociationsResponse = v3.AssociationsResponse{
		Associations: []string{associationName},
	}
	expectedAssociationResponse = associationValue

	bridgeTask = &apitask.Task{
		Arn:                      taskARN,
		Associations:             []apitask.Association{association},
		Family:                   family,
		Version:                  version,
		DesiredStatusUnsafe:      apitaskstatus.TaskRunning,
		KnownStatusUnsafe:        apitaskstatus.TaskRunning,
		CPU:                      cpu,
		Memory:                   memory,
		PullStartedAtUnsafe:      now,
		PullStoppedAtUnsafe:      now,
		ExecutionStoppedAtUnsafe: now,
		LaunchType:               "EC2",
	}
	container1 = &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		KnownPortBindingsUnsafe: []apicontainer.PortBinding{
			{
				ContainerPort: aws.Uint16(containerPort),
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
		NetworkModeUnsafe: bridgeMode,
		NetworkSettingsUnsafe: &types.NetworkSettings{
			DefaultNetworkSettings: types.DefaultNetworkSettings{
				IPAddress: bridgeIPAddr,
			},
		},
	}
	bridgeContainer = &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container1,
	}
	containerNameToBridgeContainer = map[string]*apicontainer.DockerContainer{
		taskARN: bridgeContainer,
	}
	expectedBridgeContainerResponse = v2.ContainerResponse{
		ID:            containerID,
		Name:          containerName,
		DockerName:    containerName,
		Image:         imageName,
		ImageID:       imageID,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Limits: v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		Type:   containerType,
		Labels: labels,
		Ports: []v1.PortResponse{
			{
				ContainerPort: containerPort,
				Protocol:      containerPortProtocol,
			},
		},
		Networks: []containermetadata.Network{
			{
				NetworkMode:   bridgeMode,
				IPv4Addresses: []string{bridgeIPAddr},
			},
		},
	}
	expectedBridgeTaskResponse = v2.TaskResponse{
		Cluster:       clusterName,
		TaskARN:       taskARN,
		Family:        family,
		Revision:      version,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Containers:    []v2.ContainerResponse{expectedBridgeContainerResponse},
		Limits: &v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		PullStartedAt:      aws.Time(now.UTC()),
		PullStoppedAt:      aws.Time(now.UTC()),
		ExecutionStoppedAt: aws.Time(now.UTC()),
		AvailabilityZone:   availabilityzone,
	}
	attachmentIndexVar          = attachmentIndex
	expectedV4ContainerResponse = v4.ContainerResponse{
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
			Ports: []v1.PortResponse{
				{
					ContainerPort: containerPort,
					Protocol:      containerPortProtocol,
					HostPort:      containerPort,
				},
			},
			Networks: []containermetadata.Network{
				{
					NetworkMode:   utils.NetworkModeAWSVPC,
					IPv4Addresses: []string{eniIPv4Address},
				},
			},
		},
		Networks: []v4.Network{{
			Network: containermetadata.Network{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: []string{eniIPv4Address},
			},
			NetworkInterfaceProperties: v4.NetworkInterfaceProperties{
				AttachmentIndex:          &attachmentIndexVar,
				IPV4SubnetCIDRBlock:      iPv4SubnetCIDRBlock,
				MACAddress:               macAddress,
				PrivateDNSName:           privateDNSName,
				SubnetGatewayIPV4Address: subnetGatewayIpv4Address,
			}},
		},
	}
	expectedV4PulledContainerResponse = v4.ContainerResponse{
		ContainerResponse: &v2.ContainerResponse{
			Name:          pulledContainerName,
			Image:         imageName,
			ImageID:       imageID,
			DesiredStatus: statusRunning,
			KnownStatus:   statusPulled,
			ContainerARN:  "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
			Limits: v2.LimitsResponse{
				CPU:    aws.Float64(cpu),
				Memory: aws.Int64(memory),
			},
			Type: containerType,
		},
	}
	expectedV4TaskResponse = v4.TaskResponse{
		TaskResponse: &v2.TaskResponse{
			Cluster:       clusterName,
			TaskARN:       taskARN,
			Family:        family,
			Revision:      version,
			DesiredStatus: statusRunning,
			KnownStatus:   statusRunning,
			Containers:    []v2.ContainerResponse{expectedContainerResponse},
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
		Containers: []v4.ContainerResponse{expectedV4ContainerResponse},
		VPCID:      vpcID,
	}
	expectedV4PulledTaskResponse = v4.TaskResponse{
		TaskResponse: &v2.TaskResponse{
			Cluster:       clusterName,
			TaskARN:       taskARN,
			Family:        family,
			Revision:      version,
			DesiredStatus: statusRunning,
			KnownStatus:   statusNone,
			Containers:    []v2.ContainerResponse{expectedContainerResponse, expectedPulledContainerResponse},
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
		Containers: []v4.ContainerResponse{expectedV4ContainerResponse, expectedV4PulledContainerResponse},
		VPCID:      vpcID,
	}
	expectedV4BridgeContainerResponse = v4.ContainerResponse{
		ContainerResponse: &expectedBridgeContainerResponse,
		Networks: []v4.Network{{
			Network: containermetadata.Network{
				NetworkMode:   bridgeMode,
				IPv4Addresses: []string{bridgeIPAddr},
			},
			NetworkInterfaceProperties: v4.NetworkInterfaceProperties{
				AttachmentIndex:          nil,
				IPV4SubnetCIDRBlock:      "",
				MACAddress:               "",
				PrivateDNSName:           "",
				SubnetGatewayIPV4Address: "",
			}},
		},
	}
	expectedV4BridgeTaskResponse = v4.TaskResponse{
		TaskResponse: &v2.TaskResponse{
			Cluster:       clusterName,
			TaskARN:       taskARN,
			Family:        family,
			Revision:      version,
			DesiredStatus: statusRunning,
			KnownStatus:   statusRunning,
			Containers:    []v2.ContainerResponse{expectedBridgeContainerResponse},
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
		Containers: []v4.ContainerResponse{expectedV4BridgeContainerResponse},
		VPCID:      vpcID,
	}
)

func init() {
	container.SetLabels(labels)
	container1.SetLabels(labels)
}

// TestInvalidPath tests if HTTP status code 404 is returned when invalid path is queried.
func TestInvalidPath(t *testing.T) {
	testErrorResponsesFromServer(t, "/", nil)
}

// TestCredentialsV1RequestWithNoArguments tests if HTTP status code 400 is returned when
// query parameters are not specified for the credentials endpoint.
func TestCredentialsV1RequestWithNoArguments(t *testing.T) {
	msg := &utils.ErrorMessage{
		Code:          v1.ErrNoIDInRequest,
		Message:       "CredentialsV1Request: No ID in the request",
		HTTPErrorCode: http.StatusBadRequest,
	}
	testErrorResponsesFromServer(t, credentials.V1CredentialsPath, msg)
}

// TestCredentialsV2RequestWithNoArguments tests if HTTP status code 400 is returned when
// query parameters are not specified for the credentials endpoint.
func TestCredentialsV2RequestWithNoArguments(t *testing.T) {
	msg := &utils.ErrorMessage{
		Code:          v1.ErrNoIDInRequest,
		Message:       "CredentialsV2Request: No ID in the request",
		HTTPErrorCode: http.StatusBadRequest,
	}
	testErrorResponsesFromServer(t, credentials.V2CredentialsPath+"/", msg)
}

// TestCredentialsV1RequestWhenCredentialsIdNotFound tests if HTTP status code 400 is returned when
// the credentials manager does not contain the credentials id specified in the query.
func TestCredentialsV1RequestWhenCredentialsIdNotFound(t *testing.T) {
	expectedErrorMessage := &utils.ErrorMessage{
		Code:          v1.ErrInvalidIDInRequest,
		Message:       fmt.Sprintf("CredentialsV1Request: Credentials not found"),
		HTTPErrorCode: http.StatusBadRequest,
	}
	path := credentials.V1CredentialsPath + "?id=" + credentialsID
	_, err := getResponseForCredentialsRequest(t, expectedErrorMessage.HTTPErrorCode,
		expectedErrorMessage, path, func() (credentials.TaskIAMRoleCredentials, bool) { return credentials.TaskIAMRoleCredentials{}, false })
	assert.NoError(t, err, "Error getting response body")
}

// TestCredentialsV2RequestWhenCredentialsIdNotFound tests if HTTP status code 400 is returned when
// the credentials manager does not contain the credentials id specified in the query.
func TestCredentialsV2RequestWhenCredentialsIdNotFound(t *testing.T) {
	expectedErrorMessage := &utils.ErrorMessage{
		Code:          v1.ErrInvalidIDInRequest,
		Message:       fmt.Sprintf("CredentialsV2Request: Credentials not found"),
		HTTPErrorCode: http.StatusBadRequest,
	}
	path := credentials.V2CredentialsPath + "/" + credentialsID
	_, err := getResponseForCredentialsRequest(t, expectedErrorMessage.HTTPErrorCode,
		expectedErrorMessage, path, func() (credentials.TaskIAMRoleCredentials, bool) { return credentials.TaskIAMRoleCredentials{}, false })
	assert.NoError(t, err, "Error getting response body")
}

// TestCredentialsV1RequestWhenCredentialsUninitialized tests if HTTP status code 500 is returned when
// the credentials manager returns empty credentials.
func TestCredentialsV1RequestWhenCredentialsUninitialized(t *testing.T) {
	expectedErrorMessage := &utils.ErrorMessage{
		Code:          v1.ErrCredentialsUninitialized,
		Message:       fmt.Sprintf("CredentialsV1Request: Credentials uninitialized for ID"),
		HTTPErrorCode: http.StatusServiceUnavailable,
	}
	path := credentials.V1CredentialsPath + "?id=" + credentialsID
	_, err := getResponseForCredentialsRequest(t, expectedErrorMessage.HTTPErrorCode,
		expectedErrorMessage, path, func() (credentials.TaskIAMRoleCredentials, bool) { return credentials.TaskIAMRoleCredentials{}, true })
	assert.NoError(t, err, "Error getting response body")
}

// TestCredentialsV2RequestWhenCredentialsUninitialized tests if HTTP status code 500 is returned when
// the credentials manager returns empty credentials.
func TestCredentialsV2RequestWhenCredentialsUninitialized(t *testing.T) {
	expectedErrorMessage := &utils.ErrorMessage{
		Code:          v1.ErrCredentialsUninitialized,
		Message:       fmt.Sprintf("CredentialsV2Request: Credentials uninitialized for ID"),
		HTTPErrorCode: http.StatusServiceUnavailable,
	}
	path := credentials.V2CredentialsPath + "/" + credentialsID
	_, err := getResponseForCredentialsRequest(t, expectedErrorMessage.HTTPErrorCode,
		expectedErrorMessage, path, func() (credentials.TaskIAMRoleCredentials, bool) { return credentials.TaskIAMRoleCredentials{}, true })
	assert.NoError(t, err, "Error getting response body")
}

// TestCredentialsV1RequestWhenCredentialsFound tests if HTTP status code 200 is returned when
// the credentials manager contains the credentials id specified in the query.
func TestCredentialsV1RequestWhenCredentialsFound(t *testing.T) {
	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			RoleArn:         roleArn,
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
	}
	path := credentials.V1CredentialsPath + "?id=" + credentialsID
	body, err := getResponseForCredentialsRequest(t, http.StatusOK, nil, path, func() (credentials.TaskIAMRoleCredentials, bool) { return creds, true })
	assert.NoError(t, err)

	credentials, err := parseResponseBody(body)
	assert.NoError(t, err, "Error retrieving credentials")

	assert.Equal(t, roleArn, credentials.RoleArn, "Incorrect credentials received: role ARN")
	assert.Equal(t, accessKeyID, credentials.AccessKeyID, "Incorrect credentials received: access key ID")
	assert.Equal(t, secretAccessKey, credentials.SecretAccessKey, "Incorrect credentials received: secret access key")
}

// TestCredentialsV2RequestWhenCredentialsFound tests if HTTP status code 200 is returned when
// the credentials manager contains the credentials id specified in the query.
func TestCredentialsV2RequestWhenCredentialsFound(t *testing.T) {
	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			RoleArn:         roleArn,
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
	}
	path := credentials.V2CredentialsPath + "/" + credentialsID
	body, err := getResponseForCredentialsRequest(t, http.StatusOK, nil, path, func() (credentials.TaskIAMRoleCredentials, bool) { return creds, true })
	if err != nil {
		t.Fatalf("Error retrieving credentials response: %v", err)
	}

	credentials, err := parseResponseBody(body)
	assert.NoError(t, err, "Error retrieving credentials")

	assert.Equal(t, roleArn, credentials.RoleArn, "Incorrect credentials received: role ARN")
	assert.Equal(t, accessKeyID, credentials.AccessKeyID, "Incorrect credentials received: access key ID")
	assert.Equal(t, secretAccessKey, credentials.SecretAccessKey, "Incorrect credentials received: secret access key")
}

func testErrorResponsesFromServer(t *testing.T, path string, expectedErrorMessage *utils.ErrorMessage) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	server := taskServerSetup(credentialsManager, auditLog, nil, ecsClient, "", "", nil,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, "", true)

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	if path == credentials.V1CredentialsPath || path == credentials.V2CredentialsPath+"/" {
		auditLog.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())
	}

	server.Handler.ServeHTTP(recorder, req)
	HTTPErrorCode := http.StatusNotFound
	if expectedErrorMessage != nil {
		HTTPErrorCode = expectedErrorMessage.HTTPErrorCode
	}
	assert.Equal(t, HTTPErrorCode, recorder.Code, "Incorrect return code")

	// Only paths that are equal to /v1/credentials will return valid error responses.
	if path == credentials.CredentialsPath {
		errorMessage := &utils.ErrorMessage{}
		json.Unmarshal(recorder.Body.Bytes(), errorMessage)
		assert.Equal(t, expectedErrorMessage.Code, errorMessage.Code, "Incorrect error code")
		assert.Equal(t, expectedErrorMessage.Message, errorMessage.Message, "Incorrect error message")
	}
}

// getResponseForCredentialsRequestWithParameters queries credentials for the
// given id. The getCredentials function is used to simulate getting the
// credentials object from the CredentialsManager
func getResponseForCredentialsRequest(t *testing.T, expectedStatus int,
	expectedErrorMessage *utils.ErrorMessage, path string, getCredentials func() (credentials.TaskIAMRoleCredentials, bool)) (*bytes.Buffer, error) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	server := taskServerSetup(credentialsManager, auditLog, nil, ecsClient, "", "", nil,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, "", true)
	recorder := httptest.NewRecorder()

	creds, ok := getCredentials()
	credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, ok)
	auditLog.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())

	params := make(url.Values)
	params[credentials.CredentialsIDQueryParameterName] = []string{credentialsID}

	req, _ := http.NewRequest("GET", path, nil)
	server.Handler.ServeHTTP(recorder, req)

	assert.Equal(t, expectedStatus, recorder.Code, "Incorrect return code")

	if recorder.Code != http.StatusOK {
		errorMessage := &utils.ErrorMessage{}
		json.Unmarshal(recorder.Body.Bytes(), errorMessage)

		assert.Equal(t, expectedErrorMessage.Code, errorMessage.Code, "Incorrect error code")
		assert.Equal(t, expectedErrorMessage.Message, errorMessage.Message, "Incorrect error message")
	}

	return recorder.Body, nil
}

// parseResponseBody parses the HTTP response body into an IAMRoleCredentials object.
func parseResponseBody(body *bytes.Buffer) (*credentials.IAMRoleCredentials, error) {
	response, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body: %v", err)
	}
	var creds credentials.IAMRoleCredentials
	err = json.Unmarshal(response, &creds)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling: %v", err)
	}
	return &creds, nil
}

func TestV2TaskMetadata(t *testing.T) {
	testCases := []struct {
		path string
	}{
		{
			v2BaseMetadataPath,
		},
		{
			v2BaseMetadataPath + "/",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Testing path: %s", tc.path), func(t *testing.T) {
			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			auditLog := mock_audit.NewMockAuditLogger(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)
			ecsClient := mock_api.NewMockECSClient(ctrl)

			gomock.InOrder(
				state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
				state.EXPECT().TaskByArn(taskARN).Return(task, true),
				state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
			)
			server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
				config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
				containerInstanceArn, endpoint, acceptInsecureCert)
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tc.path, nil)
			req.RemoteAddr = remoteIP + ":" + remotePort
			server.Handler.ServeHTTP(recorder, req)
			res, err := ioutil.ReadAll(recorder.Body)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var taskResponse v2.TaskResponse
			err = json.Unmarshal(res, &taskResponse)
			assert.NoError(t, err)
			assert.Equal(t, expectedTaskResponse, taskResponse)
		})
	}
}

func TestV2TaskWithTagsMetadata(t *testing.T) {
	testCases := []struct {
		path string
	}{
		{
			v2BaseMetadataWithTagsPath,
		},
		{
			v2BaseMetadataWithTagsPath + "/",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Testing path: %s", tc.path), func(t *testing.T) {
			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			auditLog := mock_audit.NewMockAuditLogger(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)
			ecsClient := mock_api.NewMockECSClient(ctrl)

			expectedTaskResponseWithTags := expectedTaskResponse
			expectedContainerInstanceTags := map[string]string{
				"ContainerInstanceTag1": "firstTag",
				"ContainerInstanceTag2": "secondTag",
			}
			expectedTaskResponseWithTags.ContainerInstanceTags = expectedContainerInstanceTags
			expectedTaskTags := map[string]string{
				"TaskTag1": "firstTag",
				"TaskTag2": "secondTag",
			}
			expectedTaskResponseWithTags.TaskTags = expectedTaskTags

			contInstTag1Key := "ContainerInstanceTag1"
			contInstTag1Val := "firstTag"
			contInstTag2Key := "ContainerInstanceTag2"
			contInstTag2Val := "secondTag"
			taskTag1Key := "TaskTag1"
			taskTag1Val := "firstTag"
			taskTag2Key := "TaskTag2"
			taskTag2Val := "secondTag"

			gomock.InOrder(
				state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
				state.EXPECT().TaskByArn(taskARN).Return(task, true),
				state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
				ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return([]*ecs.Tag{
					&ecs.Tag{
						Key:   &contInstTag1Key,
						Value: &contInstTag1Val,
					},
					&ecs.Tag{
						Key:   &contInstTag2Key,
						Value: &contInstTag2Val,
					},
				}, nil),
				ecsClient.EXPECT().GetResourceTags(taskARN).Return([]*ecs.Tag{
					&ecs.Tag{
						Key:   &taskTag1Key,
						Value: &taskTag1Val,
					},
					&ecs.Tag{
						Key:   &taskTag2Key,
						Value: &taskTag2Val,
					},
				}, nil),
			)
			server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
				config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
				containerInstanceArn, endpoint, acceptInsecureCert)
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", v2BaseMetadataWithTagsPath, nil)
			req.RemoteAddr = remoteIP + ":" + remotePort
			server.Handler.ServeHTTP(recorder, req)
			res, err := ioutil.ReadAll(recorder.Body)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var taskResponse v2.TaskResponse
			err = json.Unmarshal(res, &taskResponse)
			assert.NoError(t, err)
			assert.Equal(t, expectedTaskResponseWithTags, taskResponse)
		})
	}
}

func TestV2ContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByID(containerID).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v2BaseMetadataPath+"/"+containerID, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var containerResponse v2.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedContainerResponse, containerResponse)
}

func TestV2ContainerStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	dockerStats := &types.StatsJSON{}
	dockerStats.NumProcs = 2
	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v2BaseStatsPath+"/"+containerID, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult *types.StatsJSON
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	assert.Equal(t, dockerStats.NumProcs, statsFromResult.NumProcs)
}

func TestV2TaskStats(t *testing.T) {
	testCases := []struct {
		path string
	}{
		{
			v2BaseStatsPath,
		},
		{
			v2BaseStatsPath + "/",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Testing path: %s", tc.path), func(t *testing.T) {
			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			auditLog := mock_audit.NewMockAuditLogger(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)
			ecsClient := mock_api.NewMockECSClient(ctrl)

			dockerStats := &types.StatsJSON{}
			dockerStats.NumProcs = 2
			containerMap := map[string]*apicontainer.DockerContainer{
				containerName: {
					DockerID: containerID,
				},
			}
			gomock.InOrder(
				state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
				state.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true),
				statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
			)
			server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
				config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
				containerInstanceArn, endpoint, acceptInsecureCert)
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tc.path, nil)
			req.RemoteAddr = remoteIP + ":" + remotePort
			server.Handler.ServeHTTP(recorder, req)
			res, err := ioutil.ReadAll(recorder.Body)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var statsFromResult map[string]*types.StatsJSON
			err = json.Unmarshal(res, &statsFromResult)
			assert.NoError(t, err)
			containerStats, ok := statsFromResult[containerID]
			assert.True(t, ok)
			assert.Equal(t, dockerStats.NumProcs, containerStats.NumProcs)
		})
	}
}

func TestV3TaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v2.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedTaskResponse, taskResponse)
}

func TestV3BridgeTaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToBridgeContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v2.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedBridgeTaskResponse, taskResponse)
}

func TestV3BridgeContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
		state.EXPECT().TaskByID(containerID).Return(bridgeTask, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var containerResponse v2.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedBridgeContainerResponse, containerResponse)
}

// Test API calls for propagating Tags to Task Metadata
func TestV3TaskMetadataWithTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	expectedTaskResponseWithTags := expectedTaskResponse
	expectedContainerInstanceTags := map[string]string{
		"ContainerInstanceTag1": "firstTag",
		"ContainerInstanceTag2": "secondTag",
	}
	expectedTaskResponseWithTags.ContainerInstanceTags = expectedContainerInstanceTags
	expectedTaskTags := map[string]string{
		"TaskTag1": "firstTag",
		"TaskTag2": "secondTag",
	}
	expectedTaskResponseWithTags.TaskTags = expectedTaskTags

	contInstTag1Key := "ContainerInstanceTag1"
	contInstTag1Val := "firstTag"
	contInstTag2Key := "ContainerInstanceTag2"
	contInstTag2Val := "secondTag"
	taskTag1Key := "TaskTag1"
	taskTag1Val := "firstTag"
	taskTag2Key := "TaskTag2"
	taskTag2Val := "secondTag"

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &contInstTag1Key,
				Value: &contInstTag1Val,
			},
			&ecs.Tag{
				Key:   &contInstTag2Key,
				Value: &contInstTag2Val,
			},
		}, nil),
		ecsClient.EXPECT().GetResourceTags(taskARN).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &taskTag1Key,
				Value: &taskTag1Val,
			},
			&ecs.Tag{
				Key:   &taskTag2Key,
				Value: &taskTag2Val,
			},
		}, nil),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/taskWithTags", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v2.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedTaskResponseWithTags, taskResponse)
}

func TestV3ContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByID(containerID).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var containerResponse v2.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedContainerResponse, containerResponse)
}

func TestV3TaskStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	dockerStats := &types.StatsJSON{}
	dockerStats.NumProcs = 2

	containerMap := map[string]*apicontainer.DockerContainer{
		containerName: {
			DockerID: containerID,
		},
	}

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/task/stats", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult map[string]*types.StatsJSON
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	containerStats, ok := statsFromResult[containerID]
	assert.True(t, ok)
	assert.Equal(t, dockerStats.NumProcs, containerStats.NumProcs)
}

func TestV3ContainerStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	dockerStats := &types.StatsJSON{}
	dockerStats.NumProcs = 2

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/stats", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult *types.StatsJSON
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	assert.Equal(t, dockerStats.NumProcs, statsFromResult.NumProcs)
}

func TestV3ContainerAssociations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/associations/"+associationType, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var associationsResponse v3.AssociationsResponse
	err = json.Unmarshal(res, &associationsResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedAssociationsResponse, associationsResponse)
}

func TestV3ContainerAssociation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v3BasePath+v3EndpointID+"/associations/"+associationType+"/"+associationName, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)

	assert.Equal(t, expectedAssociationResponse, string(res))
}

func TestV4TaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes(),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes(),
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v4.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)

	expectedV4TaskResponse.TaskResponse.Containers = nil
	expectedV4ContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedV4TaskResponse, taskResponse)
}

func TestV4TaskMetadataWithPulledContainers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(pulledTask, true).AnyTimes(),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(pulledTask, true).AnyTimes(),
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(pulledContainerNameToDockerContainer, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v4.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
	expectedV4PulledTaskResponse.TaskResponse.Containers = nil
	expectedV4ContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedV4PulledTaskResponse, taskResponse)
}

func TestV4ContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByID(containerID).Return(task, true).Times(2),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var containerResponse v4.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)

	// v4.ContainerMetadata overrides Networks properties defined in v2.ContainerResponse
	// during json.Unmarshal(), values for the Networks property will be written to v4.ContainerMetadata.Networks
	// instead of v4.ContainerMetadata.(v2.ContainerMetadata).Networks
	// v2.ContainerMetadata.Networks should be nil
	expectedV4ContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedV4ContainerResponse, containerResponse)
}

// Test API calls for propagating Tags to v4 Task Metadata
func TestV4TaskMetadataWithTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	expectedv4TaskResponseWithTags := expectedV4TaskResponse
	expectedContainerInstanceTags := map[string]string{
		"ContainerInstanceTag1": "firstTag",
		"ContainerInstanceTag2": "secondTag",
	}
	expectedv4TaskResponseWithTags.ContainerInstanceTags = expectedContainerInstanceTags
	expectedTaskTags := map[string]string{
		"TaskTag1": "firstTag",
		"TaskTag2": "secondTag",
	}
	expectedv4TaskResponseWithTags.TaskTags = expectedTaskTags

	contInstTag1Key := "ContainerInstanceTag1"
	contInstTag1Val := "firstTag"
	contInstTag2Key := "ContainerInstanceTag2"
	contInstTag2Val := "secondTag"
	taskTag1Key := "TaskTag1"
	taskTag1Val := "firstTag"
	taskTag2Key := "TaskTag2"
	taskTag2Val := "secondTag"

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes(),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &contInstTag1Key,
				Value: &contInstTag1Val,
			},
			&ecs.Tag{
				Key:   &contInstTag2Key,
				Value: &contInstTag2Val,
			},
		}, nil),
		ecsClient.EXPECT().GetResourceTags(taskARN).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &taskTag1Key,
				Value: &taskTag1Val,
			},
			&ecs.Tag{
				Key:   &taskTag2Key,
				Value: &taskTag2Val,
			},
		}, nil),
		state.EXPECT().TaskByArn(taskARN).Return(task, true).AnyTimes(),
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/taskWithTags", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v4.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)

	expectedv4TaskResponseWithTags.TaskResponse.Containers = nil
	expectedV4ContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedv4TaskResponseWithTags, taskResponse)
}

func TestV4BridgeTaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToBridgeContainer, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
	)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v4.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)

	expectedV4BridgeTaskResponse.TaskResponse.Containers = nil
	expectedV4BridgeContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedV4BridgeTaskResponse, taskResponse)
}

func TestV4BridgeTaskMetadataAllowMissingContainerNetwork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToBridgeContainer, true),
		state.EXPECT().ContainerByID(containerID).Return(nil, false),
		state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
	)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, availabilityzone, vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/task", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v4.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
}

func TestV4BridgeContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
		state.EXPECT().TaskByID(containerID).Return(bridgeTask, true),
		state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true),
	)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var containerResponse v4.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)

	expectedV4BridgeContainerResponse.ContainerResponse.Networks = nil
	assert.Equal(t, expectedV4BridgeContainerResponse, containerResponse)
}

func TestV4TaskStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	dockerStats := &types.StatsJSON{}
	dockerStats.NumProcs = 2

	containerMap := map[string]*apicontainer.DockerContainer{
		containerName: {
			DockerID: containerID,
		},
	}

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/task/stats", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult map[string]*types.StatsJSON
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	containerStats, ok := statsFromResult[containerID]
	assert.True(t, ok)
	assert.Equal(t, dockerStats.NumProcs, containerStats.NumProcs)
}

func TestV4ContainerStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	dockerStats := &types.StatsJSON{}
	dockerStats.NumProcs = 2

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/stats", nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult *types.StatsJSON
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	assert.Equal(t, dockerStats.NumProcs, statsFromResult.NumProcs)
}

func TestV4ContainerAssociations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().DockerIDByV3EndpointID(v3EndpointID).Return(containerID, true),
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/associations/"+associationType, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var associationsResponse v3.AssociationsResponse
	err = json.Unmarshal(res, &associationsResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedAssociationsResponse, associationsResponse)
}

func TestV4ContainerAssociation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", v4BasePath+v3EndpointID+"/associations/"+associationType+"/"+associationName, nil)
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, expectedAssociationResponse, string(res))
}

func TestTaskHTTPEndpoint301Redirect(t *testing.T) {
	testPathsMap := map[string]string{
		"http://127.0.0.1/v3///task/":           "http://127.0.0.1/v3/task/",
		"http://127.0.0.1//v2/credentials/test": "http://127.0.0.1/v2/credentials/test",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)

	for testPath, expectedPath := range testPathsMap {
		t.Run(fmt.Sprintf("Test path: %s", testPath), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", testPath, nil)
			server.Handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusMovedPermanently, recorder.Code)
			assert.Equal(t, expectedPath, recorder.Header().Get("Location"))
		})
	}
}

func TestTaskHTTPEndpointErrorCode404(t *testing.T) {
	testPaths := []string{
		"/",
		"/v2/meta",
		"/v2/statss",
		"/v3",
		"/v3/v3-endpoint-id/",
		"/v3/v3-endpoint-id/wrong-path",
		"/v3/v3-endpoint-id/task/",
		"/v3/v3-endpoint-id/task/stats/",
		"/v3/v3-endpoint-id/task/stats/wrong-path",
		"/v3/v3-endpoint-id/associtions-with-typo/elastic-inference/dev1",
		"/v4/v3-endpoint-id/",
		"/v4/v3-endpoint-id/wrong-path",
		"/v4/v3-endpoint-id/task/",
		"/v4/v3-endpoint-id/task/stats/",
		"/v4/v3-endpoint-id/task/stats/wrong-path",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)

	for _, testPath := range testPaths {
		t.Run(fmt.Sprintf("Test path: %s", testPath), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", testPath, nil)
			server.Handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}

func TestTaskHTTPEndpointErrorCode400(t *testing.T) {
	testPaths := []string{
		"/v2/metadata",
		"/v2/metadata/",
		"/v2/metadata/wrong-container-id",
		"/v2/metadata/container-id/other-path",
		"/v2/stats",
		"/v2/stats/",
		"/v2/stats/wrong-container-id",
		"/v2/stats/container-id/other-path",
		"/v3/wrong-v3-endpoint-id/stats",
		"/v3/wrong-v3-endpoint-id/task/stats",
		"/v3/task/stats",
		"/v4/wrong-v3-endpoint-id/stats",
		"/v4/wrong-v3-endpoint-id/task/stats",
		"/v3/wrong-v3-endpoint-id/associations/elastic-inference",
		"/v3/wrong-v3-endpoint-id/associations/elastic-inference/dev1",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)

	for _, testPath := range testPaths {
		t.Run(fmt.Sprintf("Test path: %s", testPath), func(t *testing.T) {
			// Make every possible call to state fail
			state.EXPECT().TaskARNByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().DockerIDByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().TaskARNByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().GetTaskByIPAddress(gomock.Any()).Return("", false).AnyTimes()

			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", testPath, nil)
			req.RemoteAddr = remoteIP + ":" + remotePort
			server.Handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestTaskHTTPEndpointErrorCode500(t *testing.T) {
	testPaths := []string{
		"/v3/wrong-v3-endpoint-id",
		"/v3/",
		"/v3/stats",
		"/v3/wrong-v3-endpoint-id/task",
		"/v3/task",
		"/v4/wrong-v3-endpoint-id",
		"/v4/",
		"/v4/stats",
		"/v4/wrong-v3-endpoint-id/task",
		"/v4/task",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)

	for _, testPath := range testPaths {
		t.Run(fmt.Sprintf("Test path: %s", testPath), func(t *testing.T) {
			// Make every possible call to state fail
			state.EXPECT().TaskARNByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().DockerIDByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().TaskARNByV3EndpointID(gomock.Any()).Return("", false).AnyTimes()
			state.EXPECT().GetTaskByIPAddress(gomock.Any()).Return("", false).AnyTimes()

			recorder := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", testPath, nil)
			req.RemoteAddr = remoteIP + ":" + remotePort
			server.Handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		})
	}
}

// Helper function for testing Agent API Task Protection v1 handlers
func testAgentAPITaskProtectionV1Handler(t *testing.T, requestBody interface{}, method string) {
	// Prepare dependency mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	gomock.InOrder(
		state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)

	// Set up the server
	server := taskServerSetup(credentials.NewManager(), auditLog, state, ecsClient, clusterName, region, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate, "", vpcID,
		containerInstanceArn, endpoint, acceptInsecureCert)

	// Prepare the request
	var requestReader io.Reader = nil
	if requestBody != nil {
		requestBodyJSON, err := json.Marshal(requestBody)
		assert.NoError(t, err)
		requestReader = bytes.NewReader(requestBodyJSON)
	}

	// Send request and record response
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest(method, fmt.Sprintf("/api/%s/task-protection/v1/state", v3EndpointID),
		requestReader)
	server.Handler.ServeHTTP(recorder, req)

	// assert that there is response
	assert.NotNil(t, recorder.Body)
}

// Tests that Agent API v1 GetTaskProtection handler is registered correctly
func TestAgentAPIV1GetTaskProtectionHandler(t *testing.T) {
	testAgentAPITaskProtectionV1Handler(t, nil, "GET")
}

// Tests that Agent API v1 UpdateTaskProtection handler is registered correctly
func TestAgentAPIV1UpdateTaskProtectionHandler(t *testing.T) {
	requestBody := task_protection_v1.TaskProtectionRequest{
		ProtectionEnabled: agentutils.BoolPtr(false),
	}
	testAgentAPITaskProtectionV1Handler(t, requestBody, "PUT")
}
