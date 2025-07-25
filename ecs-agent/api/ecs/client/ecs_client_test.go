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

package ecsclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	mock_client "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks/client"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks/credentialsprovider"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/async"
	mock_async "github.com/aws/amazon-ecs-agent/ecs-agent/async/mocks"
	mock_config "github.com/aws/amazon-ecs-agent/ecs-agent/config/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	ecsservice "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const (
	configuredCluster    = "mycluster"
	iid                  = "instanceIdentityDocument"
	iidSignature         = "signature"
	registrationToken    = "clientToken"
	endpoint             = "some-endpoint.com"
	region               = "us-east-1"
	availabilityZone     = "us-west-2b"
	zoneId               = "use1-az1"
	defaultClusterName   = "default"
	taskARN              = "taskArn"
	containerName        = "cont"
	runtimeID            = "runtime id"
	outpostARN           = "test:arn:outpost"
	containerInstanceARN = "registerArn"
	attachmentARN        = "eniArn"
	agentVer             = "0.0.0"
	osType               = "linux"
	detailedOSFamily     = "debian_11"
)

var (
	containerInstanceTags = []types.Tag{
		{
			Key:   aws.String("my_key1"),
			Value: aws.String("my_val1"),
		},
		{
			Key:   aws.String("my_key2"),
			Value: aws.String("my_val2"),
		},
	}
	containerInstanceTagsMap = map[string]string{
		"my_key1": "my_val1",
		"my_key2": "my_val2",
	}
	testManagedAgents = []types.ManagedAgentStateChange{
		{
			ManagedAgentName: "test_managed_agent",
			ContainerName:    aws.String(containerName),
			Status:           aws.String("RUNNING"),
			Reason:           aws.String("test_reason"),
		},
	}
)

// testHelper wraps all the objects required for a test.
type testHelper struct {
	ctrl                  *gomock.Controller
	client                ecs.ECSClient
	mockStandardClient    *mock_ecs.MockECSStandardSDK
	mockSubmitStateClient *mock_ecs.MockECSSubmitStateSDK
	mockCfgAccessor       *mock_config.MockAgentConfigAccessor
}

func setup(t *testing.T,
	ctrl *gomock.Controller,
	ec2MetadataClient ec2.EC2MetadataClient,
	cfgAccessorOverrideFunc func(*mock_config.MockAgentConfigAccessor),
	options ...ECSClientOption) *testHelper {
	mockCfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	mockStandardClient := mock_ecs.NewMockECSStandardSDK(ctrl)
	mockSubmitStateClient := mock_ecs.NewMockECSSubmitStateSDK(ctrl)
	// Override the retry backoff to accelerate the test's speed, with the following backoff configuration:
	// Min backoff: 1 second; Max backoff: 1 second; Multiple: 2; Jitter: 10%.
	// This brings down the test's execution time from 20 seconds to 0.5 second.
	customRCIRetryBackoff := retry.NewExponentialBackoff(100*time.Millisecond, 200*time.Millisecond, rciRetryJitter, rciRetryMultiple)
	options = append(options, WithStandardClient(mockStandardClient),
		WithSubmitStateChangeClient(mockSubmitStateClient),
		WithRCICustomRetryBackoff(customRCIRetryBackoff))
	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), mockCfgAccessor, ec2MetadataClient, agentVer, options...)
	assert.NoError(t, err)

	return &testHelper{
		ctrl:                  ctrl,
		client:                client,
		mockStandardClient:    mockStandardClient,
		mockSubmitStateClient: mockSubmitStateClient,
		mockCfgAccessor:       mockCfgAccessor,
	}
}

func newMockConfigAccessor(ctrl *gomock.Controller,
	cfgAccessorOverrideFunc func(*mock_config.MockAgentConfigAccessor)) *mock_config.MockAgentConfigAccessor {
	cfgAccessor := mock_config.NewMockAgentConfigAccessor(ctrl)
	if cfgAccessorOverrideFunc != nil {
		cfgAccessorOverrideFunc(cfgAccessor)
	}
	applyMockCfgAccessorDefaults(cfgAccessor)
	return cfgAccessor
}

func applyMockCfgAccessorDefaults(cfgAccessor *mock_config.MockAgentConfigAccessor) {
	cfgAccessor.EXPECT().AcceptInsecureCert().Return(false).AnyTimes()
	cfgAccessor.EXPECT().APIEndpoint().Return(endpoint).AnyTimes()
	cfgAccessor.EXPECT().AWSRegion().Return(region).AnyTimes()
	cfgAccessor.EXPECT().Cluster().Return(configuredCluster).AnyTimes()
	cfgAccessor.EXPECT().DefaultClusterName().Return(defaultClusterName).AnyTimes()
	cfgAccessor.EXPECT().External().Return(false).AnyTimes()
	cfgAccessor.EXPECT().InstanceAttributes().Return(nil).AnyTimes()
	cfgAccessor.EXPECT().NoInstanceIdentityDocument().Return(false).AnyTimes()
	cfgAccessor.EXPECT().OSFamily().Return("LINUX").AnyTimes()
	cfgAccessor.EXPECT().OSFamilyDetailed().Return(detailedOSFamily).AnyTimes()
	cfgAccessor.EXPECT().OSType().Return(osType).AnyTimes()
	cfgAccessor.EXPECT().ReservedMemory().Return(uint16(20)).AnyTimes()
	cfgAccessor.EXPECT().ReservedPorts().Return([]uint16{22, 2375, 2376, 51678}).AnyTimes()
	cfgAccessor.EXPECT().ReservedPortsUDP().Return([]uint16{}).AnyTimes()
}

func TestSubmitContainerStateChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:       aws.String(configuredCluster),
		Task:          aws.String(taskARN),
		ContainerName: aws.String(containerName),
		RuntimeId:     aws.String(runtimeID),
		Status:        aws.String("RUNNING"),
		NetworkBindings: []types.NetworkBinding{
			{
				BindIP:        aws.String("1.2.3.4"),
				ContainerPort: aws.Int32(1),
				HostPort:      aws.Int32(2),
				Protocol:      "tcp",
			},
			{
				BindIP:        aws.String("2.2.3.4"),
				ContainerPort: aws.Int32(3),
				HostPort:      aws.Int32(4),
				Protocol:      "udp",
			},
			{
				BindIP:             aws.String("5.6.7.8"),
				ContainerPortRange: aws.String("11-12"),
				HostPortRange:      aws.String("11-12"),
				Protocol:           "udp",
			},
		},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:       taskARN,
		ContainerName: containerName,
		RuntimeID:     runtimeID,
		Status:        apicontainerstatus.ContainerRunning,
		NetworkBindings: []types.NetworkBinding{
			{
				BindIP:        aws.String("1.2.3.4"),
				ContainerPort: aws.Int32(1),
				HostPort:      aws.Int32(2),
				Protocol:      "tcp",
			},
			{
				BindIP:        aws.String("2.2.3.4"),
				ContainerPort: aws.Int32(3),
				HostPort:      aws.Int32(4),
				Protocol:      "udp",
			},
			{
				BindIP:             aws.String("5.6.7.8"),
				ContainerPortRange: aws.String("11-12"),
				HostPortRange:      aws.String("11-12"),
				Protocol:           "udp",
			},
		},
	})

	assert.NoError(t, err, "Unable to submit container state change")
}

func TestSubmitContainerStateChangeFull(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	exitCode := 20
	reason := "I exited"

	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:         aws.String(configuredCluster),
		Task:            aws.String(taskARN),
		ContainerName:   aws.String(containerName),
		RuntimeId:       aws.String(runtimeID),
		Status:          aws.String("STOPPED"),
		ExitCode:        aws.Int32(int32(exitCode)),
		Reason:          aws.String(reason),
		NetworkBindings: []types.NetworkBinding{},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:         taskARN,
		ContainerName:   containerName,
		RuntimeID:       runtimeID,
		Status:          apicontainerstatus.ContainerStopped,
		ExitCode:        &exitCode,
		Reason:          reason,
		NetworkBindings: []types.NetworkBinding{},
	})

	assert.NoError(t, err, "Unable to submit container state change")
}

func TestSubmitContainerStateChangeReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	exitCode := 20
	reason := strings.Repeat("a", ecsMaxContainerReasonLength)

	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:         aws.String(configuredCluster),
		Task:            aws.String(taskARN),
		ContainerName:   aws.String(containerName),
		Status:          aws.String("STOPPED"),
		ExitCode:        aws.Int32(int32(exitCode)),
		Reason:          aws.String(reason),
		NetworkBindings: []types.NetworkBinding{},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:         taskARN,
		ContainerName:   containerName,
		Status:          apicontainerstatus.ContainerStopped,
		ExitCode:        &exitCode,
		Reason:          reason,
		NetworkBindings: []types.NetworkBinding{},
	})

	assert.NoError(t, err)
}

func TestSubmitContainerStateChangeLongReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	exitCode := 20
	trimmedReason := strings.Repeat("a", ecsMaxContainerReasonLength)
	reason := strings.Repeat("a", ecsMaxContainerReasonLength+1)

	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:         aws.String(configuredCluster),
		Task:            aws.String(taskARN),
		ContainerName:   aws.String(containerName),
		Status:          aws.String("STOPPED"),
		ExitCode:        aws.Int32(int32(exitCode)),
		Reason:          aws.String(trimmedReason),
		NetworkBindings: []types.NetworkBinding{},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:         taskARN,
		ContainerName:   containerName,
		Status:          apicontainerstatus.ContainerStopped,
		ExitCode:        &exitCode,
		Reason:          reason,
		NetworkBindings: []types.NetworkBinding{},
	})

	assert.NoError(t, err, "Unable to submit container state change")
}

func TestSetInstanceIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const (
		expectedIID    = "test-instance-identity-document"
		expectedIIDSig = "test-instance-identity-signature"
	)

	testCases := []struct {
		name                   string
		noInstanceIdentity     bool
		mockEC2MetadataSetup   func(*mock_ec2.MockEC2MetadataClient)
		mockCredentialsSetup   func(*mock_credentials.MockCredentialsProvider)
		validateExpectedFields func(*testing.T, *ecsservice.RegisterContainerInstanceInput)
	}{
		{
			name:               "NoInstanceIdentity_True",
			noInstanceIdentity: true,
			mockEC2MetadataSetup: func(mockEC2Metadata *mock_ec2.MockEC2MetadataClient) {
				// No calls expected when noInstanceIdentity is true
			},
			mockCredentialsSetup: func(mockCredProvider *mock_credentials.MockCredentialsProvider) {
				// No credential operations expected
			},
			validateExpectedFields: func(t *testing.T, req *ecsservice.RegisterContainerInstanceInput) {
				assert.NotNil(t, req.InstanceIdentityDocument)
				assert.Equal(t, "", *req.InstanceIdentityDocument)
				assert.NotNil(t, req.InstanceIdentityDocumentSignature)
				assert.Equal(t, "", *req.InstanceIdentityDocumentSignature)
			},
		},
		{
			name:               "Success_FirstAttempt",
			noInstanceIdentity: false,
			mockEC2MetadataSetup: func(mockEC2Metadata *mock_ec2.MockEC2MetadataClient) {
				gomock.InOrder(
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return(expectedIID, nil),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
						Return(expectedIIDSig, nil),
				)
			},
			mockCredentialsSetup: func(mockCredProvider *mock_credentials.MockCredentialsProvider) {
				// No credential operations expected for successful attempt
			},
			validateExpectedFields: func(t *testing.T, req *ecsservice.RegisterContainerInstanceInput) {
				assert.NotNil(t, req.InstanceIdentityDocument)
				assert.Equal(t, expectedIID, *req.InstanceIdentityDocument)
				assert.NotNil(t, req.InstanceIdentityDocumentSignature)
				assert.Equal(t, expectedIIDSig, *req.InstanceIdentityDocumentSignature)
			},
		},
		{
			name:               "Success_AfterRetry",
			noInstanceIdentity: false,
			mockEC2MetadataSetup: func(mockEC2Metadata *mock_ec2.MockEC2MetadataClient) {
				gomock.InOrder(
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return("", fmt.Errorf("temporary error")),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return(expectedIID, nil),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
						Return(expectedIIDSig, nil),
				)
			},
			mockCredentialsSetup: func(mockCredProvider *mock_credentials.MockCredentialsProvider) {
				// Expect credential retrieval after first failure
				mockCredProvider.EXPECT().Retrieve(gomock.Any()).Return(
					aws.Credentials{}, nil).Times(1)
			},
			validateExpectedFields: func(t *testing.T, req *ecsservice.RegisterContainerInstanceInput) {
				assert.NotNil(t, req.InstanceIdentityDocument)
				assert.Equal(t, expectedIID, *req.InstanceIdentityDocument)
				assert.NotNil(t, req.InstanceIdentityDocumentSignature)
				assert.Equal(t, expectedIIDSig, *req.InstanceIdentityDocumentSignature)
			},
		},
		{
			name:               "Fail_PersistentError",
			noInstanceIdentity: false,
			mockEC2MetadataSetup: func(mockEC2Metadata *mock_ec2.MockEC2MetadataClient) {
				// All calls to GetDynamicData fail - allow any number of retries
				mockEC2Metadata.EXPECT().
					GetDynamicData(ec2.InstanceIdentityDocumentResource).
					Return("", fmt.Errorf("persistent error")).
					AnyTimes()
				mockEC2Metadata.EXPECT().
					GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
					Return("", fmt.Errorf("persistent error")).
					AnyTimes()
			},
			mockCredentialsSetup: func(mockCredProvider *mock_credentials.MockCredentialsProvider) {
				// Expect multiple credential retrievals due to repeated failures
				mockCredProvider.EXPECT().Retrieve(gomock.Any()).Return(
					aws.Credentials{}, nil).MinTimes(1)
			},
			validateExpectedFields: func(t *testing.T, req *ecsservice.RegisterContainerInstanceInput) {
				assert.NotNil(t, req.InstanceIdentityDocument)
				assert.Equal(t, "", *req.InstanceIdentityDocument)
				assert.NotNil(t, req.InstanceIdentityDocumentSignature)
				assert.Equal(t, "", *req.InstanceIdentityDocumentSignature)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockConfigAccessor := mock_config.NewMockAgentConfigAccessor(ctrl)
			mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mockCredentialsProvider := mock_credentials.NewMockCredentialsProvider(ctrl)
			mockConfigAccessor.EXPECT().AcceptInsecureCert().Return(false).AnyTimes()
			mockConfigAccessor.EXPECT().OSType().Return(osType).AnyTimes()
			mockConfigAccessor.EXPECT().OSFamilyDetailed().Return(detailedOSFamily).AnyTimes()
			mockConfigAccessor.EXPECT().AWSRegion().Return(region).AnyTimes()
			mockConfigAccessor.EXPECT().NoInstanceIdentityDocument().Return(tc.noInstanceIdentity).AnyTimes()
			tc.mockEC2MetadataSetup(mockEC2Metadata)
			tc.mockCredentialsSetup(mockCredentialsProvider)

			registerRequest := ecsservice.RegisterContainerInstanceInput{
				Cluster: aws.String("test-cluster"),
			}

			client := &ecsClient{
				credentialsCache: aws.NewCredentialsCache(mockCredentialsProvider),
				configAccessor:   mockConfigAccessor,
				ec2metadata:      mockEC2Metadata,
			}

			result := client.setInstanceIdentity(registerRequest)
			tc.validateExpectedFields(t, &result)
		})
	}
}
func buildAttributeList(capabilities []string, attributes map[string]string) []types.Attribute {
	var rv []types.Attribute
	for _, capability := range capabilities {
		rv = append(rv, types.Attribute{Name: aws.String(capability)})
	}
	for key, value := range attributes {
		rv = append(rv, types.Attribute{Name: aws.String(key), Value: aws.String(value)})
	}
	return rv
}

func TestRegisterContainerInstance(t *testing.T) {
	testCases := []struct {
		name                    string
		mockCfgAccessorOverride func(cfgAccessor *mock_config.MockAgentConfigAccessor)
	}{
		{
			name:                    "basic case",
			mockCfgAccessorOverride: nil,
		},
		{
			name:                    "retry GetDynamicData",
			mockCfgAccessorOverride: nil,
		},
		{
			name: "no instance identity doc",
			mockCfgAccessorOverride: func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
				cfgAccessor.EXPECT().NoInstanceIdentityDocument().Return(true).AnyTimes()
			},
		},
		{
			name: "on prem",
			mockCfgAccessorOverride: func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
				cfgAccessor.EXPECT().NoInstanceIdentityDocument().Return(true).AnyTimes()
				cfgAccessor.EXPECT().External().Return(true).AnyTimes()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
			additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
				"my_other_custom_attribute": "Custom_Value2",
			}
			cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
				cfgAccessor.EXPECT().InstanceAttributes().Return(additionalAttributes).AnyTimes()
				if tc.mockCfgAccessorOverride != nil {
					tc.mockCfgAccessorOverride(cfgAccessor)
				}
			}
			tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc)

			fakeCapabilities := []string{"capability1", "capability2"}
			expectedAttributes := map[string]string{
				"ecs.os-type":               tester.mockCfgAccessor.OSType(),
				"ecs.os-family":             tester.mockCfgAccessor.OSFamily(),
				"my_custom_attribute":       "Custom_Value1",
				"my_other_custom_attribute": "Custom_Value2",
				"ecs.availability-zone":     availabilityZone,
				"ecs.outpost-arn":           outpostARN,
				cpuArchAttrName:             getCPUArch(),
			}
			capabilities := buildAttributeList(fakeCapabilities, nil)
			platformDevices := []types.PlatformDevice{
				{
					Id:   aws.String("id1"),
					Type: types.PlatformDeviceTypeGpu,
				},
				{
					Id:   aws.String("id2"),
					Type: types.PlatformDeviceTypeGpu,
				},
				{
					Id:   aws.String("id3"),
					Type: types.PlatformDeviceTypeGpu,
				},
			}

			expectedIID := iid
			expectedIIDSig := iidSignature
			if tester.mockCfgAccessor.NoInstanceIdentityDocument() {
				expectedIID = ""
				expectedIIDSig = ""
			} else if tc.name == "retry GetDynamicData" {
				gomock.InOrder(
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return("", errors.New("fake unit test error")),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return(expectedIID, nil),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
						Return("", errors.New("fake unit test error")),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
						Return(expectedIIDSig, nil),
				)
			} else {
				// Basic case.
				gomock.InOrder(
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
						Return(expectedIID, nil),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
						Return(expectedIIDSig, nil),
				)
			}

			var expectedNumOfAttributes int
			if !tester.mockCfgAccessor.External() {
				// 2 capability attributes: capability1, capability2
				// and 5 other attributes:
				// ecs.os-type, ecs.os-family, ecs.outpost-arn, my_custom_attribute, my_other_custom_attribute.
				expectedNumOfAttributes = 7
			} else {
				// One more attribute for external case: ecs.cpu-architecture.
				expectedNumOfAttributes = 8
			}

			gomock.InOrder(
				tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
					Do(func(_ context.Context, req *ecsservice.RegisterContainerInstanceInput, _ ...func(*ecsservice.Options)) {
						assert.Nil(t, req.ContainerInstanceArn)
						assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
						assert.Equal(t, registrationToken, *req.ClientToken, "Wrong client token")
						assert.Equal(t, expectedIID, *req.InstanceIdentityDocument, "Wrong IID")
						assert.Equal(t, expectedIIDSig, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
						assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
						resource, ok := findResource(req.TotalResources, "PORTS_UDP")
						require.True(t, ok, `Could not find resource "PORTS_UDP"`)
						assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
						assert.Equal(t, expectedNumOfAttributes, len(req.Attributes), "Wrong number of Attributes")
						attrs := attributesToMap(req.Attributes)
						for name, value := range attrs {
							if strings.Contains(name, "capability") {
								assert.Contains(t, fakeCapabilities, name)
							} else {
								assert.Equal(t, expectedAttributes[name], value)
							}
						}
						assert.Equal(t, len(containerInstanceTags), len(req.Tags), "Wrong number of tags")
						assert.Equal(t, len(platformDevices), len(req.PlatformDevices), "Wrong number of devices")
						reqTags := extractTagsMapFromRegisterContainerInstanceInput(req)
						for k, v := range reqTags {
							assert.Contains(t, containerInstanceTagsMap, k)
							assert.Equal(t, containerInstanceTagsMap[k], v)
						}
					}).Return(&ecsservice.RegisterContainerInstanceOutput{
					ContainerInstance: &types.ContainerInstance{
						ContainerInstanceArn: aws.String(containerInstanceARN),
						Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes)}},
					nil),
			)

			arn, availabilityzone, err := tester.client.RegisterContainerInstance("", capabilities,
				containerInstanceTags, registrationToken, platformDevices, outpostARN)
			require.NoError(t, err)
			assert.Equal(t, containerInstanceARN, arn)
			assert.Equal(t, availabilityZone, availabilityzone)
		})
	}
}

// TestRegisterContainerInstanceWithRetry tests the RegisterContainerInstanceWithRetry wrapper.
// The wrapper implements registerContainerInstance with an additional layer of retry with exponential backoff.
// RCI call has 4 failure types: ServerException, ClientException, InvalidParameterException, ThrottlingException.
// ServerException and ThrottlingException are considered transient, and should be retried upon receiving such failures.
// We have 2 subtests, one for happy path and one for unhappy path.
// For both tests, we will make 3 RCI calls, and the first 2 will fail with ThrottlingException and ServerException, respectively.
// In the happy test, the last RCI call will succeed, and we will examine the expected attributes are present.
// In the unhappy test, the last RCI call will fail with ClientException, and an appropriate error will be returned.
// For both test cases, the last RCI call should effectively terminate the retry loop.
func TestRegisterContainerInstanceWithRetryNonTerminalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().InstanceAttributes().Return(additionalAttributes).AnyTimes()
	}
	tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc)

	fakeCapabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.os-type":               tester.mockCfgAccessor.OSType(),
		"ecs.os-family":             tester.mockCfgAccessor.OSFamily(),
		"my_custom_attribute":       "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
		"ecs.availability-zone":     availabilityZone,
		"ecs.outpost-arn":           outpostARN,
		cpuArchAttrName:             getCPUArch(),
	}
	capabilities := buildAttributeList(fakeCapabilities, nil)
	platformDevices := []types.PlatformDevice{
		{
			Id:   aws.String("id1"),
			Type: types.PlatformDeviceTypeGpu,
		},
		{
			Id:   aws.String("id2"),
			Type: types.PlatformDeviceTypeGpu,
		},
		{
			Id:   aws.String("id3"),
			Type: types.PlatformDeviceTypeGpu,
		},
	}

	testCases := []struct {
		name                         string
		finalRCICallResponse         *ecsservice.RegisterContainerInstanceOutput
		finalRCICallError            error
		expectedContainerInstanceARN string
		expectedAZ                   string
	}{
		{
			name: "Happy Path",
			finalRCICallResponse: &ecsservice.RegisterContainerInstanceOutput{
				ContainerInstance: &types.ContainerInstance{
					ContainerInstanceArn: aws.String(containerInstanceARN),
					Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes)},
			},
			finalRCICallError:            nil,
			expectedContainerInstanceARN: containerInstanceARN,
			expectedAZ:                   availabilityZone,
		},
		{
			name:                 "UnHappy Path",
			finalRCICallResponse: nil,
			finalRCICallError:    &types.ClientException{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gomock.InOrder(
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
					Return("instanceIdentityDocument", nil),
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
					Return("signature", nil),
				tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
					Return(nil, &types.LimitExceededException{}),
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
					Return("instanceIdentityDocument", nil),
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
					Return("signature", nil),
				tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
					Return(nil, &types.ServerException{}),
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
					Return("instanceIdentityDocument", nil),
				mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
					Return("signature", nil),
				tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
					Return(tc.finalRCICallResponse, tc.finalRCICallError),
			)
			arn, availabilityzone, err := tester.client.RegisterContainerInstance("", capabilities,
				containerInstanceTags, registrationToken, platformDevices, outpostARN)
			assert.Equal(t, tc.finalRCICallError, err)
			assert.Equal(t, tc.expectedContainerInstanceARN, arn)
			assert.Equal(t, tc.expectedAZ, availabilityzone)
		})
	}
}

func TestReRegisterContainerInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute":    "Custom_Value2",
		"attribute_name_with_no_value": "",
	}
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().InstanceAttributes().Return(additionalAttributes).AnyTimes()
	}
	tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc)

	fakeCapabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.os-type":           tester.mockCfgAccessor.OSType(),
		"ecs.os-family":         tester.mockCfgAccessor.OSFamily(),
		"ecs.availability-zone": availabilityZone,
		"ecs.outpost-arn":       outpostARN,
	}
	for i := range fakeCapabilities {
		expectedAttributes[fakeCapabilities[i]] = ""
	}
	capabilities := buildAttributeList(fakeCapabilities, nil)

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, req *ecsservice.RegisterContainerInstanceInput, _ ...func(*ecsservice.Options)) {
				assert.Equal(t, "arn:test", *req.ContainerInstanceArn, "Wrong container instance ARN")
				assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
				assert.Equal(t, registrationToken, *req.ClientToken, "Wrong client token")
				assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
				assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
				assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
				resource, ok := findResource(req.TotalResources, "PORTS_UDP")
				assert.True(t, ok, `Could not find resource "PORTS_UDP"`)
				assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
				// "ecs.os-type", ecs.os-family, ecs.outpost-arn and the 2 that we specified as additionalAttributes.
				assert.Equal(t, 5, len(req.Attributes), "Wrong number of Attributes")
				reqAttributes := func() map[string]string {
					rv := make(map[string]string, len(req.Attributes))
					for i := range req.Attributes {
						rv[aws.ToString(req.Attributes[i].Name)] = aws.ToString(req.Attributes[i].Value)
					}
					return rv
				}()
				for k, v := range reqAttributes {
					assert.Contains(t, expectedAttributes, k)
					assert.Equal(t, expectedAttributes[k], v)
				}
				assert.Equal(t, len(containerInstanceTags), len(req.Tags), "Wrong number of tags")
				reqTags := extractTagsMapFromRegisterContainerInstanceInput(req)
				for k, v := range reqTags {
					assert.Contains(t, containerInstanceTagsMap, k)
					assert.Equal(t, containerInstanceTagsMap[k], v)
				}
			}).Return(&ecsservice.RegisterContainerInstanceOutput{
			ContainerInstance: &types.ContainerInstance{
				ContainerInstanceArn: aws.String(containerInstanceARN),
				Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes),
			}},
			nil),
	)

	arn, availabilityzone, err := tester.client.RegisterContainerInstance("arn:test", capabilities,
		containerInstanceTags, registrationToken, nil, outpostARN)

	assert.NoError(t, err)
	assert.Equal(t, containerInstanceARN, arn)
	assert.Equal(t, availabilityZone, availabilityzone, "availabilityZone is incorrect")
}

// TestRegisterContainerInstanceWithNegativeResource tests the registration should fail with negative resource.
func TestRegisterContainerInstanceWithNegativeResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mem := 1024
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().ReservedMemory().Return(uint16(mem) + 1).AnyTimes()
	}
	tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc,
		WithAvailableMemoryProvider(func() int32 {
			return int32(mem)
		}))

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
	)
	_, _, err := tester.client.RegisterContainerInstance("", nil, nil,
		"", nil, "")
	assert.ErrorContains(t, err, "reserved memory is higher than available memory",
		"Register resource with negative value should cause registration fail")
}

func TestRegisterContainerInstanceWithEmptyTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	tester := setup(t, ctrl, mockEC2Metadata, nil)

	expectedAttributes := map[string]string{
		"ecs.os-type":               tester.mockCfgAccessor.OSType(),
		"ecs.os-family":             tester.mockCfgAccessor.OSFamily(),
		"my_custom_attribute":       "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}
	fakeCapabilities := []string{"capability1", "capability2"}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, req *ecsservice.RegisterContainerInstanceInput, _ ...func(*ecsservice.Options)) {
				assert.Nil(t, req.Tags)
			}).Return(&ecsservice.RegisterContainerInstanceOutput{
			ContainerInstance: &types.ContainerInstance{
				ContainerInstanceArn: aws.String(containerInstanceARN),
				Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes)}},
			nil),
	)

	_, _, err := tester.client.RegisterContainerInstance("", nil, make([]types.Tag, 0),
		"", nil, "")
	assert.NoError(t, err)
}

func TestValidateRegisteredAttributes(t *testing.T) {
	origAttributes := []types.Attribute{
		{Name: aws.String("foo"), Value: aws.String("bar")},
		{Name: aws.String("baz"), Value: aws.String("quux")},
		{Name: aws.String("no_value"), Value: aws.String("")},
	}
	actualAttributes := []types.Attribute{
		{Name: aws.String("baz"), Value: aws.String("quux")},
		{Name: aws.String("foo"), Value: aws.String("bar")},
		{Name: aws.String("no_value"), Value: aws.String("")},
		{Name: aws.String("ecs.internal-attribute"), Value: aws.String("some text")},
	}
	assert.NoError(t, validateRegisteredAttributes(origAttributes, actualAttributes))

	origAttributes = append(origAttributes, types.Attribute{Name: aws.String("abc"), Value: aws.String("xyz")})
	assert.ErrorContains(t, validateRegisteredAttributes(origAttributes, actualAttributes),
		"Attribute validation failed")
}

func findResource(resources []types.Resource, name string) (types.Resource, bool) {
	for _, resource := range resources {
		if name == *resource.Name {
			return resource, true
		}
	}
	return types.Resource{}, false
}

func TestRegisterBlankCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	// Test the special 'empty cluster' behavior of creating 'default'.
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().Cluster().Return("").AnyTimes()
	}
	tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc)

	expectedAttributes := map[string]string{
		"ecs.os-type":   tester.mockCfgAccessor.OSType(),
		"ecs.os-family": tester.mockCfgAccessor.OSFamily(),
	}
	defaultCluster := tester.mockCfgAccessor.DefaultClusterName()
	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Return(nil, &types.ClusterNotFoundException{Message: aws.String("Cluster not found.")}),
		tester.mockStandardClient.EXPECT().CreateCluster(gomock.Any(), &ecsservice.CreateClusterInput{ClusterName: &defaultCluster}).
			Return(&ecsservice.CreateClusterOutput{Cluster: &types.Cluster{ClusterName: &defaultCluster}}, nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, req *ecsservice.RegisterContainerInstanceInput, _ ...func(*ecsservice.Options)) {
				assert.Equal(t, defaultCluster, *req.Cluster, "Wrong cluster")
				assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
				assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			}).Return(&ecsservice.RegisterContainerInstanceOutput{
			ContainerInstance: &types.ContainerInstance{
				ContainerInstanceArn: aws.String(containerInstanceARN),
				Attributes:           buildAttributeList(nil, expectedAttributes)}},
			nil),
		tester.mockCfgAccessor.EXPECT().UpdateCluster(defaultCluster),
	)

	arn, availabilityzone, err := tester.client.RegisterContainerInstance("", nil, nil,
		"", nil, "")
	assert.NoError(t, err, "Should not be an error")
	assert.Equal(t, containerInstanceARN, arn, "Wrong arn")
	assert.Empty(t, availabilityzone, "wrong availability zone")
}

func TestRegisterBlankClusterNotCreatingClusterWhenErrorNotClusterNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	// Test the special 'empty cluster' behavior of creating 'default'.
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().Cluster().Return("").AnyTimes()
	}
	tester := setup(t, ctrl, mockEC2Metadata, cfgAccessorOverrideFunc)

	expectedAttributes := map[string]string{
		"ecs.os-type":   tester.mockCfgAccessor.OSType(),
		"ecs.os-family": tester.mockCfgAccessor.OSFamily(),
	}

	defaultCluster := tester.mockCfgAccessor.DefaultClusterName()
	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Return(nil, &types.InvalidParameterException{Message: aws.String("Invalid request.")}),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).
			Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).
			Return("signature", nil),
		tester.mockStandardClient.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, req *ecsservice.RegisterContainerInstanceInput, _ ...func(*ecsservice.Options)) {
				assert.Equal(t, defaultCluster, *req.Cluster, "Wrong cluster")
				assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
				assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			}).Return(&ecsservice.RegisterContainerInstanceOutput{
			ContainerInstance: &types.ContainerInstance{
				ContainerInstanceArn: aws.String(containerInstanceARN),
				Attributes:           buildAttributeList(nil, expectedAttributes)}},
			nil),
		tester.mockCfgAccessor.EXPECT().UpdateCluster(defaultCluster),
	)

	arn, _, err := tester.client.RegisterContainerInstance("", nil, nil, "",
		nil, "")
	assert.NoError(t, err, "Should not return error")
	assert.Equal(t, containerInstanceARN, arn, "Wrong arn")
}

func TestDiscoverTelemetryEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	expectedEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{TelemetryEndpoint: &expectedEndpoint}, nil)
	endpoint, err := tester.client.DiscoverTelemetryEndpoint(containerInstanceARN)
	assert.NoError(t, err, "Error getting telemetry endpoint")
	assert.Equal(t, expectedEndpoint, endpoint, "Expected telemetry endpoint != endpoint")
}

func TestDiscoverTelemetryEndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(nil,
		fmt.Errorf("Error getting endpoint"))
	_, err := tester.client.DiscoverTelemetryEndpoint(containerInstanceARN)
	assert.ErrorContains(t, err, "Error getting endpoint",
		"Expected error getting telemetry endpoint, didn't get any")
}

func TestDiscoverNilTelemetryEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := tester.client.DiscoverTelemetryEndpoint(containerInstanceARN)
	assert.ErrorContains(t, err, "no telemetry endpoint returned",
		"Expected error getting telemetry endpoint with old response")
}

func TestDiscoverServiceConnectEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	expectedEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{ServiceConnectEndpoint: &expectedEndpoint}, nil)
	endpoint, err := tester.client.DiscoverServiceConnectEndpoint(containerInstanceARN)
	assert.NoError(t, err, "Error getting service connect endpoint")
	assert.Equal(t, expectedEndpoint, endpoint, "Expected telemetry endpoint != endpoint")
}

func TestDiscoverServiceConnectEndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(nil,
		fmt.Errorf("Error getting endpoint"))
	_, err := tester.client.DiscoverServiceConnectEndpoint(containerInstanceARN)
	assert.ErrorContains(t, err, "Error getting endpoint",
		"Expected error getting service connect endpoint, didn't get any")
}

func TestDiscoverNilServiceConnectEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := tester.client.DiscoverServiceConnectEndpoint(containerInstanceARN)
	assert.ErrorContains(t, err, "no ServiceConnect endpoint returned",
		"Expected error getting service connect endpoint with old response")
}

func TestDiscoverSystemLogsEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	expectedEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{SystemLogsEndpoint: &expectedEndpoint}, nil)
	endpoint, err := tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, zoneId)
	assert.NoError(t, err, "Error getting system logs endpoint")
	assert.Equal(t, expectedEndpoint, endpoint, "Expected system logs endpoint != endpoint")
}

func TestDiscoverSystemLogsEndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(nil,
		fmt.Errorf("Error getting endpoint"))
	_, err := tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, zoneId)
	assert.ErrorContains(t, err, "Error getting endpoint",
		"Expected error getting system logs endpoint, didn't get any")
}

func TestDiscoverNilSystemLogsEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).
		Return(&ecsservice.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, zoneId)
	assert.ErrorContains(t, err, "no system logs endpoint returned",
		"Expected error getting system logs endpoint with old response")
}

func TestDiscoverPollEndpointRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	// SDK call to DiscoverPollEndpoint should only happen once.
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Times(1).Do(func(interface{}, interface{}, ...interface{}) {
		// Wait before returning to try and induce the race condition.
		time.Sleep(100 * time.Millisecond)
	}).Return(&ecsservice.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)

	var wg sync.WaitGroup
	wg.Add(2)

	// First caller.
	go func() {
		defer wg.Done()
		tester.client.DiscoverPollEndpoint(containerInstanceARN)
	}()
	// Second caller.
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		tester.client.DiscoverPollEndpoint(containerInstanceARN)
	}()

	wg.Wait()
}

func TestUpdateContainerInstancesState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	status := types.ContainerInstanceStatusDraining
	tester.mockStandardClient.EXPECT().UpdateContainerInstancesState(gomock.Any(), &ecsservice.UpdateContainerInstancesStateInput{
		ContainerInstances: []string{containerInstanceARN},
		Status:             status,
		Cluster:            aws.String(configuredCluster),
	}).Return(&ecsservice.UpdateContainerInstancesStateOutput{}, nil)

	err := tester.client.UpdateContainerInstancesState(containerInstanceARN, status)
	assert.NoError(t, err, fmt.Sprintf("Unexpected error calling UpdateContainerInstancesState: %s", err))
}

func TestUpdateContainerInstancesStateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	status := types.ContainerInstanceStatusDraining
	tester.mockStandardClient.EXPECT().UpdateContainerInstancesState(gomock.Any(), &ecsservice.UpdateContainerInstancesStateInput{
		ContainerInstances: []string{containerInstanceARN},
		Status:             status,
		Cluster:            aws.String(configuredCluster),
	}).Return(nil, fmt.Errorf("ERROR"))

	err := tester.client.UpdateContainerInstancesState(containerInstanceARN, status)
	assert.ErrorContains(t, err, "ERROR",
		"Expected an error calling UpdateContainerInstancesState but got nil")
}

func TestGetResourceTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
		ResourceArn: aws.String(containerInstanceARN),
	}).Return(&ecsservice.ListTagsForResourceOutput{
		Tags: containerInstanceTags,
	}, nil)

	tags, err := tester.client.GetResourceTags(containerInstanceARN)
	assert.NoError(t, err, fmt.Sprintf("Unexpected error calling GetResourceTags: %s", err))
	assert.Equal(t, containerInstanceTags, tags, "Unexpected tags returned")
}

func TestGetResourceTagsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
		ResourceArn: aws.String(containerInstanceARN),
	}).Return(nil, fmt.Errorf("ERROR")).MinTimes(1)

	_, err := tester.client.GetResourceTags(containerInstanceARN)

	assert.Error(t, err, "Expected GetResourceTags to fail after retries")
	assert.ErrorContains(t, err, "GetResourceTags failed for",
		"Expected GetResourceTags to fail after retries")
	assert.ErrorContains(t, err, "attempts",
		"Expected error message to include attempt count")
}

// TestGetResourceTagsWithRetry tests the exponential backoff retry functionality
// for both transient and non-transient errors.
func TestGetResourceTagsWithRetry(t *testing.T) {
	testCases := []struct {
		name          string
		errors        []error
		finalResponse *ecsservice.ListTagsForResourceOutput
		finalError    error
		expectSuccess bool
		expectedTags  []types.Tag
		expectedCalls int
	}{
		{
			name: "Success after throttling retries",
			errors: []error{
				mock_client.NewThrottlingException(),
				mock_client.NewLimitExceededException(),
			},
			finalResponse: &ecsservice.ListTagsForResourceOutput{
				Tags: containerInstanceTags,
			},
			finalError:    nil,
			expectSuccess: true,
			expectedTags:  containerInstanceTags,
			expectedCalls: 3,
		},
		{
			name: "Success after server exception retries",
			errors: []error{
				mock_client.NewServerException(),
			},
			finalResponse: &ecsservice.ListTagsForResourceOutput{
				Tags: containerInstanceTags,
			},
			finalError:    nil,
			expectSuccess: true,
			expectedTags:  containerInstanceTags,
			expectedCalls: 2,
		},
		{
			name: "Success after mixed transient errors",
			errors: []error{
				mock_client.NewThrottlingException(),
				mock_client.NewServerException(),
				mock_client.NewQuotaExceededError(),
			},
			finalResponse: &ecsservice.ListTagsForResourceOutput{
				Tags: containerInstanceTags,
			},
			finalError:    nil,
			expectSuccess: true,
			expectedTags:  containerInstanceTags,
			expectedCalls: 4,
		},
		{
			name: "Non-transient error - retries until timeout",
			errors: []error{
				mock_client.NewInvalidParameterException(),
			},
			finalResponse: nil,
			finalError:    mock_client.NewInvalidParameterException(),
			expectSuccess: false,
			expectedCalls: 10, // Will retry until timeout
		},
		{
			name: "Non-transient error - client exception retries until timeout",
			errors: []error{
				mock_client.NewClientException(),
			},
			finalResponse: nil,
			finalError:    mock_client.NewClientException(),
			expectSuccess: false,
			expectedCalls: 10, // Will retry until timeout
		},
		{
			name: "Success after multiple throttling retries",
			errors: []error{
				mock_client.NewThrottlingException(),
				mock_client.NewThrottlingException(),
				mock_client.NewThrottlingException(),
				mock_client.NewThrottlingException(),
			},
			finalResponse: &ecsservice.ListTagsForResourceOutput{
				Tags: containerInstanceTags,
			},
			finalError:    nil,
			expectSuccess: true,
			expectedTags:  containerInstanceTags,
			expectedCalls: 5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

			if tc.expectSuccess {
				for _, err := range tc.errors {
					tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
						ResourceArn: aws.String(containerInstanceARN),
					}).Return(nil, err)
				}
				tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
					ResourceArn: aws.String(containerInstanceARN),
				}).Return(tc.finalResponse, tc.finalError).Times(1)
			} else {
				if tc.name == "Exhausted retries" {
					tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
						ResourceArn: aws.String(containerInstanceARN),
					}).Return(nil, tc.errors[0]).AnyTimes()
				} else {
					if len(tc.errors) > 0 {
						tester.mockStandardClient.EXPECT().ListTagsForResource(gomock.Any(), &ecsservice.ListTagsForResourceInput{
							ResourceArn: aws.String(containerInstanceARN),
						}).Return(nil, tc.errors[0]).MinTimes(1)
					}
				}
			}

			tags, err := tester.client.GetResourceTags(containerInstanceARN)

			if tc.expectSuccess {
				assert.NoError(t, err, "Expected GetResourceTags to succeed")
				assert.Equal(t, tc.expectedTags, tags, "Unexpected tags returned")
			} else {
				assert.Error(t, err, "Expected GetResourceTags to fail")
				assert.Nil(t, tags, "Expected no tags on failure")

				if len(tc.errors) == 1 && tc.name != "Exhausted retries" {
					assert.ErrorContains(t, err, "GetResourceTags failed for", "Expected wrapped error message")
				}
			}
		})
	}
}

func TestDiscoverPollEndpointCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := mock_async.NewMockTTLCache(ctrl)
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	pollEndpointCache.EXPECT().Get(containerInstanceARN).Return(
		&ecsservice.DiscoverPollEndpointOutput{
			Endpoint: aws.String(pollEndpoint),
		}, false, true)
	output, err := tester.client.(*ecsClient).discoverPollEndpoint(containerInstanceARN, "")
	assert.NoError(t, err, "Error in discoverPollEndpoint")
	assert.Equal(t, pollEndpoint, aws.ToString(output.Endpoint), "Mismatch in poll endpoint")
}

func TestDiscoverPollEndpointCacheMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := mock_async.NewMockTTLCache(ctrl)
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	pollEndpointOutput := &ecsservice.DiscoverPollEndpointOutput{
		Endpoint: &pollEndpoint,
	}

	gomock.InOrder(
		pollEndpointCache.EXPECT().Get(containerInstanceARN).Return(nil, false, false),
		tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(pollEndpointOutput, nil),
		pollEndpointCache.EXPECT().Set(containerInstanceARN, pollEndpointOutput),
	)

	output, err := tester.client.(*ecsClient).discoverPollEndpoint(containerInstanceARN, "")
	assert.NoError(t, err, "Error in discoverPollEndpoint")
	assert.Equal(t, pollEndpoint, aws.ToString(output.Endpoint), "Mismatch in poll endpoint")
}

func TestDiscoverPollEndpointExpiredButDPEFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := mock_async.NewMockTTLCache(ctrl)
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	pollEndpointOutput := &ecsservice.DiscoverPollEndpointOutput{
		Endpoint: &pollEndpoint,
	}

	gomock.InOrder(
		pollEndpointCache.EXPECT().Get(containerInstanceARN).Return(pollEndpointOutput, true, false),
		tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("error!")),
	)

	output, err := tester.client.(*ecsClient).discoverPollEndpoint(containerInstanceARN, "")
	assert.NoError(t, err, "Error in discoverPollEndpoint")
	assert.Equal(t, pollEndpoint, aws.ToString(output.Endpoint),
		"Mismatch in poll endpoint")
}

func TestDiscoverTelemetryEndpointAfterPollEndpointCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := async.NewTTLCache(&async.TTL{Duration: 10 * time.Minute})
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(
		&ecsservice.DiscoverPollEndpointOutput{
			Endpoint:          &pollEndpoint,
			TelemetryEndpoint: &pollEndpoint,
		}, nil)
	endpoint, err := tester.client.DiscoverPollEndpoint(containerInstanceARN)
	assert.NoError(t, err, "Error in DiscoverPollEndpoint")
	assert.Equal(t, pollEndpoint, endpoint, "Mismatch in poll endpoint")

	telemetryEndpoint, err := tester.client.DiscoverTelemetryEndpoint(containerInstanceARN)
	assert.NoError(t, err, "Error in discoverTelemetryEndpoint")
	assert.Equal(t, pollEndpoint, telemetryEndpoint, "Mismatch in telemetry endpoint")
}

// TestDiscoverSystemLogsEndpointAfterPollEndpointCacheHit_HappyPath refers to the scenario
// where the system logs endpoint is present in the cache, therefore *not* requiring another
// DiscoverPollEndpoint invocation.
func TestDiscoverSystemLogsEndpointAfterCacheHit_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := async.NewTTLCache(&async.TTL{Duration: 10 * time.Minute})
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(
		&ecsservice.DiscoverPollEndpointOutput{
			Endpoint:           &pollEndpoint,
			SystemLogsEndpoint: &pollEndpoint,
		}, nil).Times(1)
	systemLogsEndpoint, err := tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, availabilityZone)
	assert.NoError(t, err, "Error in DiscoverSystemLogsEndpoint")
	assert.Equal(t, pollEndpoint, systemLogsEndpoint, "Mismatch in system logs endpoint")

	systemLogsEndpoint, err = tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, availabilityZone)
	assert.NoError(t, err, "Error in DiscoverSystemLogsEndpoint")
	assert.Equal(t, pollEndpoint, systemLogsEndpoint, "Mismatch in system logs endpoint")
}

// TestDiscoverSystemLogsEndpointAfterPollEndpointCacheHit_UnhappyPath refers to the scenario
// where the system logs endpoint is missing from the cache, requiring a DiscoverPollEndpoint invocation.
func TestDiscoverSystemLogsEndpointAfterPollEndpointCacheHit_UnhappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pollEndpointCache := async.NewTTLCache(&async.TTL{Duration: 10 * time.Minute})
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithDiscoverPollEndpointCache(pollEndpointCache))
	pollEndpoint := "http://127.0.0.1"
	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(
		&ecsservice.DiscoverPollEndpointOutput{
			Endpoint: &pollEndpoint,
		}, nil).Times(1)
	endpoint, err := tester.client.DiscoverPollEndpoint(containerInstanceARN)
	assert.NoError(t, err, "Error in DiscoverPollEndpoint")
	assert.Equal(t, pollEndpoint, endpoint, "Mismatch in poll endpoint")

	tester.mockStandardClient.EXPECT().DiscoverPollEndpoint(gomock.Any(), gomock.Any()).Return(
		&ecsservice.DiscoverPollEndpointOutput{
			Endpoint:           &pollEndpoint,
			SystemLogsEndpoint: &pollEndpoint,
		}, nil).Times(1)
	systemLogsEndpoint, err := tester.client.DiscoverSystemLogsEndpoint(containerInstanceARN, availabilityZone)
	assert.NoError(t, err, "Error in DiscoverSystemLogsEndpoint")
	assert.Equal(t, pollEndpoint, systemLogsEndpoint, "Mismatch in system logs endpoint")
}

// TestSubmitTaskStateChangeWithAttachments tests the SubmitTaskStateChange API
// also send the Attachment Status.
func TestSubmitTaskStateChangeWithAttachments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockSubmitStateClient.EXPECT().SubmitTaskStateChange(gomock.Any(), &ecsservice.SubmitTaskStateChangeInput{
		Cluster: aws.String(configuredCluster),
		Task:    aws.String(taskARN),
		Attachments: []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(attachmentARN),
				Status:        aws.String("ATTACHED"),
			},
		},
	})

	err := tester.client.SubmitTaskStateChange(ecs.TaskStateChange{
		TaskARN: taskARN,
		Attachment: &ni.ENIAttachment{
			AttachmentInfo: attachment.AttachmentInfo{
				AttachmentARN: attachmentARN,
				Status:        attachment.AttachmentAttached,
			},
		},
	})
	assert.NoError(t, err, "Unable to submit task state change with attachments")
}

func TestSubmitTaskStateChangeWithoutAttachments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockSubmitStateClient.EXPECT().SubmitTaskStateChange(gomock.Any(), &ecsservice.SubmitTaskStateChangeInput{
		Cluster: aws.String(configuredCluster),
		Task:    aws.String(taskARN),
		Reason:  aws.String(""),
		Status:  aws.String("RUNNING"),
	})

	err := tester.client.SubmitTaskStateChange(ecs.TaskStateChange{
		TaskARN: taskARN,
		Status:  apitaskstatus.TaskRunning,
	})
	assert.NoError(t, err, "Unable to submit task state change with no attachments")
}

func TestSubmitTaskStateChangeWithManagedAgents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	tester.mockSubmitStateClient.EXPECT().SubmitTaskStateChange(gomock.Any(), &ecsservice.SubmitTaskStateChangeInput{
		Cluster:       aws.String(configuredCluster),
		Task:          aws.String(taskARN),
		Reason:        aws.String(""),
		Status:        aws.String("RUNNING"),
		ManagedAgents: testManagedAgents,
	})

	err := tester.client.SubmitTaskStateChange(ecs.TaskStateChange{
		TaskARN:       taskARN,
		Status:        apitaskstatus.TaskRunning,
		ManagedAgents: testManagedAgents,
	})
	assert.NoError(t, err, "Unable to submit task state change with managed agents")
}

// TestSubmitContainerStateChangeWhileTaskInPending tests the container state change was submitted
// when the task is still in pending state.
func TestSubmitContainerStateChangeWhileTaskInPending(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCases := []struct {
		taskStatus apitaskstatus.TaskStatus
	}{
		{
			apitaskstatus.TaskStatusNone,
		},
		{
			apitaskstatus.TaskPulled,
		},
		{
			apitaskstatus.TaskCreated,
		},
	}

	taskStateChangePending := ecs.TaskStateChange{
		Status:  apitaskstatus.TaskCreated,
		TaskARN: taskARN,
		Containers: []types.ContainerStateChange{
			{
				ContainerName:   aws.String(containerName),
				RuntimeId:       aws.String(runtimeID),
				Status:          aws.String(apicontainerstatus.ContainerRunning.String()),
				NetworkBindings: []types.NetworkBinding{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("TaskStatus: %s", tc.taskStatus.String()), func(t *testing.T) {
			taskStateChangePending.Status = tc.taskStatus
			tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)
			tester.mockSubmitStateClient.EXPECT().SubmitTaskStateChange(gomock.Any(), &ecsservice.SubmitTaskStateChangeInput{
				Cluster: aws.String(configuredCluster),
				Task:    aws.String(taskARN),
				Status:  aws.String("PENDING"),
				Reason:  aws.String(""),
				Containers: []types.ContainerStateChange{
					{
						ContainerName:   aws.String(containerName),
						RuntimeId:       aws.String(runtimeID),
						Status:          aws.String("RUNNING"),
						NetworkBindings: []types.NetworkBinding{},
					},
				},
			})
			err := tester.client.SubmitTaskStateChange(taskStateChangePending)
			assert.NoError(t, err)
		})
	}
}

func TestSubmitAttachmentStateChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	tester.mockSubmitStateClient.EXPECT().SubmitAttachmentStateChanges(gomock.Any(), &ecsservice.SubmitAttachmentStateChangesInput{
		Cluster: aws.String(configuredCluster),
		Attachments: []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(attachmentARN),
				Status:        aws.String("ATTACHED"),
			},
		},
	})
	err := tester.client.SubmitAttachmentStateChange(ecs.AttachmentStateChange{
		Attachment: &ni.ENIAttachment{
			AttachmentInfo: attachment.AttachmentInfo{
				AttachmentARN: attachmentARN,
				Status:        attachment.AttachmentAttached,
			},
		},
	})

	assert.NoError(t, err, "Unable to submit attachment state change")
}

func TestSubmitAttachmentStateChangeWithRetriableError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sascCustomRetryBackoffCalled := false
	sascCustomRetryBackoff := func(fn func() error) error {
		sascCustomRetryBackoffCalled = true
		return retry.RetryWithBackoff(retry.NewExponentialBackoff(
			100*time.Millisecond, 100*time.Millisecond, 0, 1), fn)
	}
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithSASCCustomRetryBackoff(sascCustomRetryBackoff))

	input := &ecsservice.SubmitAttachmentStateChangesInput{
		Cluster: aws.String(configuredCluster),
		Attachments: []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(attachmentARN),
				Status:        aws.String("ATTACHED"),
			},
		},
	}

	// Ensure that we try to submit attachment state change twice (i.e., retried on retriable error).
	retriableError := errors.New("retriable error")
	tester.mockSubmitStateClient.EXPECT().SubmitAttachmentStateChanges(gomock.Any(), input).Return(
		nil, retriableError)
	tester.mockSubmitStateClient.EXPECT().SubmitAttachmentStateChanges(gomock.Any(), input)

	err := tester.client.SubmitAttachmentStateChange(ecs.AttachmentStateChange{
		Attachment: &ni.ENIAttachment{
			AttachmentInfo: attachment.AttachmentInfo{
				AttachmentARN: attachmentARN,
				Status:        attachment.AttachmentAttached,
			},
		},
	})

	assert.True(t, sascCustomRetryBackoffCalled)
	assert.NoError(t, err, "Unable to submit attachment state change")
}

func TestSubmitAttachmentStateChangeWithNonRetriableError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sascCustomRetryBackoffCalled := false
	sascCustomRetryBackoff := func(fn func() error) error {
		sascCustomRetryBackoffCalled = true
		return retry.RetryWithBackoff(retry.NewExponentialBackoff(
			100*time.Millisecond, 100*time.Millisecond, 0, 1), fn)
	}
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithSASCCustomRetryBackoff(sascCustomRetryBackoff))

	input := &ecsservice.SubmitAttachmentStateChangesInput{
		Cluster: aws.String(configuredCluster),
		Attachments: []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(attachmentARN),
				Status:        aws.String("ATTACHED"),
			},
		},
	}

	// Ensure that we try to submit attachment state change only once (i.e., not retried).
	nonRetriableError := &types.InvalidParameterException{}
	tester.mockSubmitStateClient.EXPECT().SubmitAttachmentStateChanges(gomock.Any(), input).Return(
		nil, nonRetriableError)

	err := tester.client.SubmitAttachmentStateChange(ecs.AttachmentStateChange{
		Attachment: &ni.ENIAttachment{
			AttachmentInfo: attachment.AttachmentInfo{
				AttachmentARN: attachmentARN,
				Status:        attachment.AttachmentAttached,
			},
		},
	})

	assert.True(t, sascCustomRetryBackoffCalled)
	assert.Error(t, err,
		"Received no error submitting attachment state change but expected to receive non-retriable error")
}

func TestAddDefaultSchemeToEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	schemelessEndpoint := "schemeless-endpoint.com"
	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return(schemelessEndpoint).AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.NotEmpty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer)
	assert.NoError(t, err)

	clientEndpoint := client.(*ecsClient).standardClient.(*ecsservice.Client).Options().BaseEndpoint
	assert.NotNil(t, clientEndpoint)
	assert.Equal(t, "https://"+schemelessEndpoint, *clientEndpoint)
}

func TestFIPSEndpointStateWhenEndpointGiven(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Endpoint is given during the call to newMockConfigAccessor function.
	cfgAccessor := newMockConfigAccessor(ctrl, nil)
	assert.NotEmpty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer)
	assert.NoError(t, err)
	assert.Equal(t, aws.FIPSEndpointStateUnset,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseFIPSEndpoint)
}

func TestFIPSEndpointStateOnFIPSEnabledHosts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return("").AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.Empty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(), agentVer, WithFIPSDetected(true))
	assert.NoError(t, err)
	assert.Equal(t, aws.FIPSEndpointStateEnabled,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseFIPSEndpoint)
}

func TestFIPSEndpointStateOnFIPSDisabledHosts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return("").AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.Empty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer)
	assert.NoError(t, err)
	assert.Equal(t, aws.FIPSEndpointStateUnset,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseFIPSEndpoint)
}

func TestDualStackEndpointStateWhenEndpointGiven(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Endpoint is given during the call to newMockConfigAccessor function.
	cfgAccessor := newMockConfigAccessor(ctrl, nil)
	assert.NotEmpty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer)
	assert.NoError(t, err)
	assert.Equal(t, aws.DualStackEndpointStateUnset,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseDualStackEndpoint)
}

func TestDualStackEndpointStateEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return("").AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.Empty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer, WithDualStackEnabled(true))
	assert.NoError(t, err)
	assert.Equal(t, aws.DualStackEndpointStateEnabled,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseDualStackEndpoint)
}

func TestDualStackEndpointStateDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return("").AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.Empty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer)
	assert.NoError(t, err)
	assert.Equal(t, aws.DualStackEndpointStateUnset,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseDualStackEndpoint)
}

func TestDualStackAndFIPSEndpointStatesEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfgAccessorOverrideFunc := func(cfgAccessor *mock_config.MockAgentConfigAccessor) {
		cfgAccessor.EXPECT().APIEndpoint().Return("").AnyTimes()
	}
	cfgAccessor := newMockConfigAccessor(ctrl, cfgAccessorOverrideFunc)
	assert.Empty(t, cfgAccessor.APIEndpoint())

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), cfgAccessor, ec2.NewBlackholeEC2MetadataClient(),
		agentVer, WithFIPSDetected(true), WithDualStackEnabled(true))
	assert.NoError(t, err)
	assert.Equal(t, aws.FIPSEndpointStateEnabled,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseFIPSEndpoint)
	assert.Equal(t, aws.DualStackEndpointStateEnabled,
		client.(*ecsClient).standardClient.(*ecsservice.Client).Options().EndpointOptions.UseDualStackEndpoint)
}

func TestDiscoverPollEndpointCacheTTLSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ttlDuration := time.Minute
	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), newMockConfigAccessor(ctrl, nil), ec2.NewBlackholeEC2MetadataClient(), agentVer, WithDiscoverPollEndpointCacheTTL(&async.TTL{Duration: ttlDuration}))
	assert.NoError(t, err)
	assert.Equal(t, ttlDuration, client.(*ecsClient).pollEndpointCache.GetTTL().Duration)
}

func TestDiscoverPollEndpointCacheTTLSetAndExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ttlDuration := time.Nanosecond
	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), newMockConfigAccessor(ctrl, nil), ec2.NewBlackholeEC2MetadataClient(), agentVer, WithDiscoverPollEndpointCacheTTL(&async.TTL{Duration: ttlDuration}))
	assert.NoError(t, err)

	client.(*ecsClient).pollEndpointCache.Set(containerInstanceARN, &ecsservice.DiscoverPollEndpointOutput{
		Endpoint: aws.String(endpoint),
	})
	time.Sleep(2 * ttlDuration)
	cachedEndpoint, expired, found := client.(*ecsClient).pollEndpointCache.Get(containerInstanceARN)

	assert.Equal(t, ttlDuration, client.(*ecsClient).pollEndpointCache.GetTTL().Duration)
	assert.True(t, found)
	assert.True(t, expired)
	assert.Equal(t, endpoint, aws.ToString(cachedEndpoint.(*ecsservice.DiscoverPollEndpointOutput).Endpoint))
}

func TestDiscoverPollEndpointCacheTTLNotSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client, err := NewECSClient(aws.NewCredentialsCache(aws.AnonymousCredentials{}), newMockConfigAccessor(ctrl, nil), ec2.NewBlackholeEC2MetadataClient(), agentVer, WithDiscoverPollEndpointCacheTTL(nil))
	assert.NoError(t, err)

	client.(*ecsClient).pollEndpointCache.Set(containerInstanceARN, &ecsservice.DiscoverPollEndpointOutput{
		Endpoint: aws.String(endpoint),
	})
	cachedEndpoint, expired, found := client.(*ecsClient).pollEndpointCache.Get(containerInstanceARN)

	assert.Nil(t, client.(*ecsClient).pollEndpointCache.GetTTL())
	assert.True(t, found)
	assert.False(t, expired)
	assert.Equal(t, endpoint, aws.ToString(cachedEndpoint.(*ecsservice.DiscoverPollEndpointOutput).Endpoint))
}

func TestPortBindingExclusions(t *testing.T) {
	ipv4PortBinding := types.NetworkBinding{
		BindIP:        aws.String("0.0.0.0"),
		ContainerPort: aws.Int32(1),
		HostPort:      aws.Int32(2),
		Protocol:      "tcp",
	}
	ipv6PortBinding := types.NetworkBinding{
		BindIP:        aws.String("::"),
		ContainerPort: aws.Int32(3),
		HostPort:      aws.Int32(4),
		Protocol:      "tcp",
	}

	testcases := []struct {
		name                    string
		clientOpts              []ECSClientOption
		expectedNetworkBindings []types.NetworkBinding
	}{
		{
			name:                    "exclude ipv4 true",
			clientOpts:              []ECSClientOption{WithIPv4PortBindingExcluded(true)},
			expectedNetworkBindings: []types.NetworkBinding{ipv6PortBinding},
		},
		{
			name:                    "exclude ipv4 false",
			clientOpts:              []ECSClientOption{WithIPv4PortBindingExcluded(false)},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding, ipv6PortBinding},
		},
		{
			name:                    "exclude ipv6 true",
			clientOpts:              []ECSClientOption{WithIPv6PortBindingExcluded(true)},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding},
		},
		{
			name:                    "exclude ipv6 false",
			clientOpts:              []ECSClientOption{WithIPv6PortBindingExcluded(false)},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding, ipv6PortBinding},
		},
		{
			name: "exclude ipv4 and ipv6",
			clientOpts: []ECSClientOption{
				WithIPv4PortBindingExcluded(true),
				WithIPv6PortBindingExcluded(true),
			},
			expectedNetworkBindings: nil,
		},
		{
			name:                    "no options set",
			clientOpts:              []ECSClientOption{},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding, ipv6PortBinding},
		},
		{
			name: "false - false",
			clientOpts: []ECSClientOption{
				WithIPv4PortBindingExcluded(false),
				WithIPv6PortBindingExcluded(false),
			},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding, ipv6PortBinding},
		},
		{
			name: "false - true",
			clientOpts: []ECSClientOption{
				WithIPv4PortBindingExcluded(false),
				WithIPv6PortBindingExcluded(true),
			},
			expectedNetworkBindings: []types.NetworkBinding{ipv4PortBinding},
		},
		{
			name: "true - false",
			clientOpts: []ECSClientOption{
				WithIPv4PortBindingExcluded(true),
				WithIPv6PortBindingExcluded(false),
			},
			expectedNetworkBindings: []types.NetworkBinding{ipv6PortBinding},
		},
		{
			name: "true - true",
			clientOpts: []ECSClientOption{
				WithIPv4PortBindingExcluded(true),
				WithIPv6PortBindingExcluded(true),
			},
			expectedNetworkBindings: nil,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil, tt.clientOpts...)

			// Test SubmitContainerStateChange
			tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(),
				&ecsservice.SubmitContainerStateChangeInput{
					Cluster:         aws.String(configuredCluster),
					Task:            aws.String(taskARN),
					ContainerName:   aws.String(containerName),
					RuntimeId:       aws.String(runtimeID),
					Status:          aws.String("RUNNING"),
					NetworkBindings: tt.expectedNetworkBindings,
				})
			err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
				TaskArn:       taskARN,
				ContainerName: containerName,
				RuntimeID:     runtimeID,
				Status:        apicontainerstatus.ContainerRunning,
				NetworkBindings: []types.NetworkBinding{
					ipv4PortBinding,
					ipv6PortBinding,
				},
			})
			assert.NoError(t, err, "Unable to submit container state change")

			// Test SubmitTaskStateChange
			tester.mockSubmitStateClient.EXPECT().SubmitTaskStateChange(gomock.Any(),
				&ecsservice.SubmitTaskStateChangeInput{
					Cluster: aws.String(configuredCluster),
					Task:    aws.String(taskARN),
					Status:  aws.String("RUNNING"),
					Reason:  aws.String(""),
					Containers: []types.ContainerStateChange{
						{
							ContainerName:   aws.String(containerName),
							RuntimeId:       aws.String(runtimeID),
							Status:          aws.String("RUNNING"),
							NetworkBindings: tt.expectedNetworkBindings,
						},
					}})
			err = tester.client.SubmitTaskStateChange(ecs.TaskStateChange{
				ClusterARN: configuredCluster,
				TaskARN:    taskARN,
				Status:     apitaskstatus.TaskRunning,
				Containers: []types.ContainerStateChange{
					{
						ContainerName: aws.String(containerName),
						RuntimeId:     aws.String(runtimeID),
						Status:        aws.String("RUNNING"),
						NetworkBindings: []types.NetworkBinding{
							ipv4PortBinding,
							ipv6PortBinding,
						},
					},
				},
			})
			assert.NoError(t, err)
		})
	}
}

func TestWithIPv6PortBindingExcludedSetTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithIPv6PortBindingExcluded(true))

	ipv4PortBinding := types.NetworkBinding{
		BindIP:        aws.String("0.0.0.0"),
		ContainerPort: aws.Int32(1),
		HostPort:      aws.Int32(2),
		Protocol:      "tcp",
	}
	ipv6PortBinding := types.NetworkBinding{
		BindIP:        aws.String("::"),
		ContainerPort: aws.Int32(3),
		HostPort:      aws.Int32(4),
		Protocol:      "tcp",
	}

	// IPv6 port binding should be excluded from container state change submitted.
	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:       aws.String(configuredCluster),
		Task:          aws.String(taskARN),
		ContainerName: aws.String(containerName),
		RuntimeId:     aws.String(runtimeID),
		Status:        aws.String("RUNNING"),
		NetworkBindings: []types.NetworkBinding{
			ipv4PortBinding,
		},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:       taskARN,
		ContainerName: containerName,
		RuntimeID:     runtimeID,
		Status:        apicontainerstatus.ContainerRunning,
		NetworkBindings: []types.NetworkBinding{
			ipv4PortBinding,
			ipv6PortBinding,
		},
	})

	assert.NoError(t, err, "Unable to submit container state change")
}

func TestWithIPv6PortBindingExcludedSetFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	ipv4PortBinding := types.NetworkBinding{
		BindIP:        aws.String("0.0.0.0"),
		ContainerPort: aws.Int32(1),
		HostPort:      aws.Int32(2),
		Protocol:      "tcp",
	}
	ipv6PortBinding := types.NetworkBinding{
		BindIP:        aws.String("::"),
		ContainerPort: aws.Int32(3),
		HostPort:      aws.Int32(4),
		Protocol:      "tcp",
	}

	// IPv6 port binding should NOT be excluded from container state change submitted.
	tester.mockSubmitStateClient.EXPECT().SubmitContainerStateChange(gomock.Any(), &ecsservice.SubmitContainerStateChangeInput{
		Cluster:       aws.String(configuredCluster),
		Task:          aws.String(taskARN),
		ContainerName: aws.String(containerName),
		RuntimeId:     aws.String(runtimeID),
		Status:        aws.String("RUNNING"),
		NetworkBindings: []types.NetworkBinding{
			ipv4PortBinding,
			ipv6PortBinding,
		},
	})
	err := tester.client.SubmitContainerStateChange(ecs.ContainerStateChange{
		TaskArn:       taskARN,
		ContainerName: containerName,
		RuntimeID:     runtimeID,
		Status:        apicontainerstatus.ContainerRunning,
		NetworkBindings: []types.NetworkBinding{
			ipv4PortBinding,
			ipv6PortBinding,
		},
	})

	assert.NoError(t, err, "Unable to submit container state change")
}

func TestTrimStringPtr(t *testing.T) {
	const testMaxLen = 32
	testCases := []struct {
		inputStringPtr *string
		expectedOutput *string
		name           string
	}{
		{nil, nil, "nil"},
		{aws.String("abc"), aws.String("abc"), "input does not exceed max length"},
		{aws.String("abcdefghijklmnopqrstuvwxyz1234567890"),
			aws.String("abcdefghijklmnopqrstuvwxyz123456"), "input exceeds max length"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, trimStringPtr(tc.inputStringPtr, testMaxLen))
		})
	}
}

func extractTagsMapFromRegisterContainerInstanceInput(req *ecsservice.RegisterContainerInstanceInput) map[string]string {
	tagsMap := make(map[string]string, len(req.Tags))
	for i := range req.Tags {
		tagsMap[aws.ToString(req.Tags[i].Key)] = aws.ToString(req.Tags[i].Value)
	}
	return tagsMap
}

func TestAvailableMemoryProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	availableMemory := int32(42)
	tester := setup(t, ctrl, ec2.NewBlackholeEC2MetadataClient(), nil,
		WithAvailableMemoryProvider(func() int32 {
			return availableMemory
		}))

	client := tester.client.(*ecsClient)
	assert.Equal(t, availableMemory, client.availableMemoryProvider())
}
