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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/async"
	mock_async "github.com/aws/amazon-ecs-agent/agent/async/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	mock_ec2 "github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const (
	configuredCluster = "mycluster"
	iid               = "instanceIdentityDocument"
	iidSignature      = "signature"
	registrationToken = "clientToken"
)

var (
	iidResponse           = []byte(iid)
	iidSignatureResponse  = []byte(iidSignature)
	containerInstanceTags = []*ecs.Tag{
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
	testManagedAgents = []*ecs.ManagedAgentStateChange{
		{
			ManagedAgentName: aws.String("test_managed_agent"),
			ContainerName:    aws.String("test_container"),
			Status:           aws.String("RUNNING"),
			Reason:           aws.String("test_reason"),
		},
	}
)

func NewMockClient(ctrl *gomock.Controller,
	ec2Metadata ec2.EC2MetadataClient,
	additionalAttributes map[string]string) (api.ECSClient, *mock_api.MockECSSDK, *mock_api.MockECSSubmitStateSDK) {

	return NewMockClientWithConfig(ctrl, ec2Metadata, additionalAttributes,
		&config.Config{
			Cluster:                      configuredCluster,
			AWSRegion:                    "us-east-1",
			InstanceAttributes:           additionalAttributes,
			ShouldExcludeIPv6PortBinding: config.BooleanDefaultTrue{Value: config.ExplicitlyEnabled},
		})
}

func NewMockClientWithConfig(ctrl *gomock.Controller,
	ec2Metadata ec2.EC2MetadataClient,
	additionalAttributes map[string]string,
	cfg *config.Config) (api.ECSClient, *mock_api.MockECSSDK, *mock_api.MockECSSubmitStateSDK) {
	client := NewECSClient(credentials.AnonymousCredentials, cfg, ec2Metadata)
	mockSDK := mock_api.NewMockECSSDK(ctrl)
	mockSubmitStateSDK := mock_api.NewMockECSSubmitStateSDK(ctrl)
	client.(*APIECSClient).SetSDK(mockSDK)
	client.(*APIECSClient).SetSubmitStateChangeSDK(mockSubmitStateSDK)
	return client, mockSDK, mockSubmitStateSDK
}

type containerSubmitInputMatcher struct {
	ecs.SubmitContainerStateChangeInput
}

type taskSubmitInputMatcher struct {
	ecs.SubmitTaskStateChangeInput
}

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }
func int64ptr(i *int) *int64 {
	if i == nil {
		return nil
	}
	j := int64(*i)
	return &j
}
func equal(lhs, rhs interface{}) bool {
	return reflect.DeepEqual(lhs, rhs)
}
func (lhs *containerSubmitInputMatcher) Matches(x interface{}) bool {
	rhs := x.(*ecs.SubmitContainerStateChangeInput)

	return (equal(lhs.Cluster, rhs.Cluster) &&
		equal(lhs.ContainerName, rhs.ContainerName) &&
		equal(lhs.ExitCode, rhs.ExitCode) &&
		equal(lhs.NetworkBindings, rhs.NetworkBindings) &&
		equal(lhs.ManagedAgents, rhs.ManagedAgents) &&
		equal(lhs.Reason, rhs.Reason) &&
		equal(lhs.Status, rhs.Status) &&
		equal(lhs.Task, rhs.Task))
}

func (lhs *containerSubmitInputMatcher) String() string {
	return fmt.Sprintf("%+v", *lhs)
}

func (lhs *taskSubmitInputMatcher) Matches(x interface{}) bool {
	rhs := x.(*ecs.SubmitTaskStateChangeInput)

	if !(equal(lhs.Cluster, rhs.Cluster) &&
		equal(lhs.Task, rhs.Task) &&
		equal(lhs.Status, rhs.Status) &&
		equal(lhs.Reason, rhs.Reason) &&
		equal(len(lhs.Attachments), len(rhs.Attachments))) {
		return false
	}

	if len(lhs.Attachments) != 0 {
		for i := range lhs.Attachments {
			if !(equal(lhs.Attachments[i].Status, rhs.Attachments[i].Status) &&
				equal(lhs.Attachments[i].AttachmentArn, rhs.Attachments[i].AttachmentArn)) {
				return false
			}
		}
	}

	if len(lhs.Containers) != 0 && !equal(lhs.Containers, rhs.Containers) {
		return false
	}

	return true
}

func (lhs *taskSubmitInputMatcher) String() string {
	return fmt.Sprintf("%+v", *lhs)
}

func TestSubmitContainerStateChange(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mockSubmitStateClient.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		ecs.SubmitContainerStateChangeInput{
			Cluster:       strptr(configuredCluster),
			Task:          strptr("arn"),
			ContainerName: strptr("cont"),
			RuntimeId:     strptr("runtime id"),
			Status:        strptr("RUNNING"),
			NetworkBindings: []*ecs.NetworkBinding{
				{
					BindIP:        strptr("1.2.3.4"),
					ContainerPort: int64ptr(intptr(1)),
					HostPort:      int64ptr(intptr(2)),
					Protocol:      strptr("tcp"),
				},
				{
					BindIP:        strptr("2.2.3.4"),
					ContainerPort: int64ptr(intptr(3)),
					HostPort:      int64ptr(intptr(4)),
					Protocol:      strptr("udp"),
				},
			},
		},
	})
	err := client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		RuntimeID:     "runtime id",
		Status:        apicontainerstatus.ContainerRunning,
		PortBindings: []apicontainer.PortBinding{
			{
				BindIP:        "1.2.3.4",
				ContainerPort: aws.Uint16(1),
				HostPort:      2,
			},
			{
				BindIP:        "2.2.3.4",
				ContainerPort: aws.Uint16(3),
				HostPort:      4,
				Protocol:      apicontainer.TransportProtocolUDP,
			},
		},
	})
	if err != nil {
		t.Errorf("Unable to submit container state change: %v", err)
	}
}

func TestSubmitContainerStateChangeFull(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	exitCode := 20
	reason := "I exited"

	mockSubmitStateClient.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		ecs.SubmitContainerStateChangeInput{
			Cluster:       strptr(configuredCluster),
			Task:          strptr("arn"),
			ContainerName: strptr("cont"),
			RuntimeId:     strptr("runtime id"),
			Status:        strptr("STOPPED"),
			ExitCode:      int64ptr(&exitCode),
			Reason:        strptr(reason),
			NetworkBindings: []*ecs.NetworkBinding{
				{
					BindIP:        strptr(""),
					ContainerPort: int64ptr(intptr(0)),
					HostPort:      int64ptr(intptr(0)),
					Protocol:      strptr("tcp"),
				},
			},
		},
	})
	err := client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		RuntimeID:     "runtime id",
		Status:        apicontainerstatus.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
		PortBindings: []apicontainer.PortBinding{
			{},
		},
	})
	if err != nil {
		t.Errorf("Unable to submit container state change: %v", err)
	}
}

func TestSubmitContainerStateChangeReason(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	exitCode := 20
	reason := strings.Repeat("a", ecsMaxContainerReasonLength)

	mockSubmitStateClient.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		ecs.SubmitContainerStateChangeInput{
			Cluster:         strptr(configuredCluster),
			Task:            strptr("arn"),
			ContainerName:   strptr("cont"),
			Status:          strptr("STOPPED"),
			ExitCode:        int64ptr(&exitCode),
			Reason:          strptr(reason),
			NetworkBindings: []*ecs.NetworkBinding{},
		},
	})
	err := client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        apicontainerstatus.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmitContainerStateChangeLongReason(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	exitCode := 20
	trimmedReason := strings.Repeat("a", ecsMaxContainerReasonLength)
	reason := strings.Repeat("a", ecsMaxContainerReasonLength+1)

	mockSubmitStateClient.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		ecs.SubmitContainerStateChangeInput{
			Cluster:         strptr(configuredCluster),
			Task:            strptr("arn"),
			ContainerName:   strptr("cont"),
			Status:          strptr("STOPPED"),
			ExitCode:        int64ptr(&exitCode),
			Reason:          strptr(trimmedReason),
			NetworkBindings: []*ecs.NetworkBinding{},
		},
	})
	err := client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        apicontainerstatus.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
	})
	if err != nil {
		t.Errorf("Unable to submit container state change: %v", err)
	}
}

func buildAttributeList(capabilities []string, attributes map[string]string) []*ecs.Attribute {
	var rv []*ecs.Attribute
	for _, capability := range capabilities {
		rv = append(rv, &ecs.Attribute{Name: aws.String(capability)})
	}
	for key, value := range attributes {
		rv = append(rv, &ecs.Attribute{Name: aws.String(key), Value: aws.String(value)})
	}
	return rv
}

func TestRegisterContainerInstance(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "basic case",
			cfg: &config.Config{
				Cluster:   configuredCluster,
				AWSRegion: "us-west-2",
			},
		},
		{
			name: "no instance identity doc",
			cfg: &config.Config{
				Cluster:   configuredCluster,
				AWSRegion: "us-west-2",
				NoIID:     true,
			},
		},
		{
			name: "on prem",
			cfg: &config.Config{
				Cluster:   configuredCluster,
				AWSRegion: "us-west-2",
				NoIID:     true,
				External:  config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
			additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
				"my_other_custom_attribute": "Custom_Value2",
			}
			tc.cfg.InstanceAttributes = additionalAttributes
			client, mc, _ := NewMockClientWithConfig(mockCtrl, mockEC2Metadata, additionalAttributes, tc.cfg)

			fakeCapabilities := []string{"capability1", "capability2"}
			expectedAttributes := map[string]string{
				"ecs.os-type":               config.OSType,
				"ecs.os-family":             config.GetOSFamily(),
				"my_custom_attribute":       "Custom_Value1",
				"my_other_custom_attribute": "Custom_Value2",
				"ecs.availability-zone":     "us-west-2b",
				"ecs.outpost-arn":           "test:arn:outpost",
				cpuArchAttrName:             getCPUArch(),
			}
			capabilities := buildAttributeList(fakeCapabilities, nil)
			platformDevices := []*ecs.PlatformDevice{
				{
					Id:   aws.String("id1"),
					Type: aws.String(ecs.PlatformDeviceTypeGpu),
				},
				{
					Id:   aws.String("id2"),
					Type: aws.String(ecs.PlatformDeviceTypeGpu),
				},
				{
					Id:   aws.String("id3"),
					Type: aws.String(ecs.PlatformDeviceTypeGpu),
				},
			}

			expectedIID := iid
			expectedIIDSig := iidSignature
			if tc.cfg.NoIID {
				expectedIID = ""
				expectedIIDSig = ""
			} else {
				gomock.InOrder(
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return(expectedIID, nil),
					mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return(expectedIIDSig, nil),
				)
			}

			var expectedNumOfAttributes int
			if !tc.cfg.External.Enabled() {
				// 2 capability attributes: capability1, capability2
				// and 5 other attributes: ecs.os-type, ecs.os-family, ecs.outpost-arn, my_custom_attribute, my_other_custom_attribute.
				expectedNumOfAttributes = 7
			} else {
				// One more attribute for external case: ecs.cpu-architecture
				expectedNumOfAttributes = 8
			}

			gomock.InOrder(
				mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
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
				}).Return(&ecs.RegisterContainerInstanceOutput{
					ContainerInstance: &ecs.ContainerInstance{
						ContainerInstanceArn: aws.String("registerArn"),
						Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes)}},
					nil),
			)

			arn, availabilityzone, err := client.RegisterContainerInstance("", capabilities,
				containerInstanceTags, registrationToken, platformDevices, "test:arn:outpost")
			require.NoError(t, err)
			assert.Equal(t, "registerArn", arn)
			assert.Equal(t, "us-west-2b", availabilityzone)
		})
	}
}

func TestReRegisterContainerInstance(t *testing.T) {
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute":    "Custom_Value2",
		"attribute_name_with_no_value": "",
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata, additionalAttributes)

	fakeCapabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.os-type":           config.OSType,
		"ecs.os-family":         config.GetOSFamily(),
		"ecs.availability-zone": "us-west-2b",
		"ecs.outpost-arn":       "test:arn:outpost",
	}
	for i := range fakeCapabilities {
		expectedAttributes[fakeCapabilities[i]] = ""
	}
	capabilities := buildAttributeList(fakeCapabilities, nil)

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			assert.Equal(t, "arn:test", *req.ContainerInstanceArn, "Wrong container instance ARN")
			assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
			assert.Equal(t, registrationToken, *req.ClientToken, "Wrong client token")
			assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
			assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
			resource, ok := findResource(req.TotalResources, "PORTS_UDP")
			assert.True(t, ok, `Could not find resource "PORTS_UDP"`)
			assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
			// "ecs.os-type", ecs.os-family, ecs.outpost-arn and the 2 that we specified as additionalAttributes
			assert.Equal(t, 5, len(req.Attributes), "Wrong number of Attributes")
			reqAttributes := func() map[string]string {
				rv := make(map[string]string, len(req.Attributes))
				for i := range req.Attributes {
					rv[aws.StringValue(req.Attributes[i].Name)] = aws.StringValue(req.Attributes[i].Value)
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
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes),
			}},
			nil),
	)

	arn, availabilityzone, err := client.RegisterContainerInstance("arn:test", capabilities,
		containerInstanceTags, registrationToken, nil, "test:arn:outpost")

	assert.NoError(t, err)
	assert.Equal(t, "registerArn", arn)
	assert.Equal(t, "us-west-2b", availabilityzone, "availabilityZone is incorrect")
}

// TestRegisterContainerInstanceWithNegativeResource tests the registeration should fail with negative resource
func TestRegisterContainerInstanceWithNegativeResource(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	_, mem := getCpuAndMemory()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client := NewECSClient(credentials.AnonymousCredentials,
		&config.Config{Cluster: configuredCluster,
			AWSRegion:      "us-east-1",
			ReservedMemory: uint16(mem) + 1,
		}, mockEC2Metadata)
	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	mockSubmitStateSDK := mock_api.NewMockECSSubmitStateSDK(mockCtrl)
	client.(*APIECSClient).SetSDK(mockSDK)
	client.(*APIECSClient).SetSubmitStateChangeSDK(mockSubmitStateSDK)

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
	)
	_, _, err := client.RegisterContainerInstance("", nil, nil,
		"", nil, "")
	assert.Error(t, err, "Register resource with negative value should cause registration fail")
}

func TestRegisterContainerInstanceWithEmptyTags(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata, nil)

	expectedAttributes := map[string]string{
		"ecs.os-type":               config.OSType,
		"ecs.os-family":             config.GetOSFamily(),
		"my_custom_attribute":       "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}

	fakeCapabilities := []string{"capability1", "capability2"}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			assert.Nil(t, req.Tags)
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(fakeCapabilities, expectedAttributes)}},
			nil),
	)

	_, _, err := client.RegisterContainerInstance("", nil, make([]*ecs.Tag, 0),
		"", nil, "")
	assert.NoError(t, err)
}

func TestValidateRegisteredAttributes(t *testing.T) {
	origAttributes := []*ecs.Attribute{
		{Name: aws.String("foo"), Value: aws.String("bar")},
		{Name: aws.String("baz"), Value: aws.String("quux")},
		{Name: aws.String("no_value"), Value: aws.String("")},
	}
	actualAttributes := []*ecs.Attribute{
		{Name: aws.String("baz"), Value: aws.String("quux")},
		{Name: aws.String("foo"), Value: aws.String("bar")},
		{Name: aws.String("no_value"), Value: aws.String("")},
		{Name: aws.String("ecs.internal-attribute"), Value: aws.String("some text")},
	}
	assert.NoError(t, validateRegisteredAttributes(origAttributes, actualAttributes))

	origAttributes = append(origAttributes, &ecs.Attribute{Name: aws.String("abc"), Value: aws.String("xyz")})
	assert.Error(t, validateRegisteredAttributes(origAttributes, actualAttributes))
}

func findResource(resources []*ecs.Resource, name string) (*ecs.Resource, bool) {
	for _, resource := range resources {
		if name == *resource.Name {
			return resource, true
		}
	}
	return nil, false
}

func TestRegisterBlankCluster(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)

	// Test the special 'empty cluster' behavior of creating 'default'
	client := NewECSClient(credentials.AnonymousCredentials,
		&config.Config{
			Cluster:   "",
			AWSRegion: "us-east-1",
		},
		mockEC2Metadata)
	mc := mock_api.NewMockECSSDK(mockCtrl)
	client.(*APIECSClient).SetSDK(mc)

	expectedAttributes := map[string]string{
		"ecs.os-type":   config.OSType,
		"ecs.os-family": config.GetOSFamily(),
	}
	defaultCluster := config.DefaultClusterName
	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Return(nil, awserr.New("ClientException", "Cluster not found.", errors.New("Cluster not found."))),
		mc.EXPECT().CreateCluster(&ecs.CreateClusterInput{ClusterName: &defaultCluster}).Return(&ecs.CreateClusterOutput{Cluster: &ecs.Cluster{ClusterName: &defaultCluster}}, nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			if *req.Cluster != config.DefaultClusterName {
				t.Errorf("Wrong cluster: %v", *req.Cluster)
			}
			if *req.InstanceIdentityDocument != iid {
				t.Errorf("Wrong IID: %v", *req.InstanceIdentityDocument)
			}
			if *req.InstanceIdentityDocumentSignature != iidSignature {
				t.Errorf("Wrong IID sig: %v", *req.InstanceIdentityDocumentSignature)
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(nil, expectedAttributes)}},
			nil),
	)

	arn, availabilityzone, err := client.RegisterContainerInstance("", nil, nil,
		"", nil, "")
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
	if availabilityzone != "" {
		t.Errorf("wrong availability zone: %v", availabilityzone)
	}
}

func TestRegisterBlankClusterNotCreatingClusterWhenErrorNotClusterNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)

	// Test the special 'empty cluster' behavior of creating 'default'
	client := NewECSClient(credentials.AnonymousCredentials,
		&config.Config{
			Cluster:   "",
			AWSRegion: "us-east-1",
		},
		mockEC2Metadata)
	mc := mock_api.NewMockECSSDK(mockCtrl)
	client.(*APIECSClient).SetSDK(mc)

	expectedAttributes := map[string]string{
		"ecs.os-type":   config.OSType,
		"ecs.os-family": config.GetOSFamily(),
	}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Return(nil, awserr.New("ClientException", "Invalid request.", errors.New("Invalid request."))),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentResource).Return("instanceIdentityDocument", nil),
		mockEC2Metadata.EXPECT().GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource).Return("signature", nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			if *req.Cluster != config.DefaultClusterName {
				t.Errorf("Wrong cluster: %v", *req.Cluster)
			}
			if *req.InstanceIdentityDocument != iid {
				t.Errorf("Wrong IID: %v", *req.InstanceIdentityDocument)
			}
			if *req.InstanceIdentityDocumentSignature != iidSignature {
				t.Errorf("Wrong IID sig: %v", *req.InstanceIdentityDocumentSignature)
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(nil, expectedAttributes)}},
			nil),
	)

	arn, _, err := client.RegisterContainerInstance("", nil, nil, "",
		nil, "")
	assert.NoError(t, err, "Should not return error")
	assert.Equal(t, "registerArn", arn, "Wrong arn")
}

func TestDiscoverTelemetryEndpoint(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	expectedEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{TelemetryEndpoint: &expectedEndpoint}, nil)
	endpoint, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err != nil {
		t.Error("Error getting telemetry endpoint: ", err)
	}
	if expectedEndpoint != endpoint {
		t.Errorf("Expected telemetry endpoint(%s) != endpoint(%s)", expectedEndpoint, endpoint)
	}
}

func TestDiscoverTelemetryEndpointError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(nil, fmt.Errorf("Error getting endpoint"))
	_, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting telemetry endpoint, didn't get any")
	}
}

func TestDiscoverNilTelemetryEndpoint(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting telemetry endpoint with old response")
	}
}

func TestDiscoverServiceConnectEndpoint(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	expectedEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{ServiceConnectEndpoint: &expectedEndpoint}, nil)
	endpoint, err := client.DiscoverServiceConnectEndpoint("containerInstance")
	if err != nil {
		t.Error("Error getting service connect endpoint: ", err)
	}
	if expectedEndpoint != endpoint {
		t.Errorf("Expected telemetry endpoint(%s) != endpoint(%s)", expectedEndpoint, endpoint)
	}
}

func TestDiscoverServiceConnectEndpointError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(nil, fmt.Errorf("Error getting endpoint"))
	_, err := client.DiscoverServiceConnectEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting service connect endpoint, didn't get any")
	}
}

func TestDiscoverNilServiceConnectEndpoint(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	pollEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := client.DiscoverServiceConnectEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting service connect endpoint with old response")
	}
}

func TestUpdateContainerInstancesState(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	instanceARN := "myInstanceARN"
	status := "DRAINING"
	mc.EXPECT().UpdateContainerInstancesState(&ecs.UpdateContainerInstancesStateInput{
		ContainerInstances: []*string{aws.String(instanceARN)},
		Status:             aws.String(status),
		Cluster:            aws.String(configuredCluster),
	}).Return(&ecs.UpdateContainerInstancesStateOutput{}, nil)

	err := client.UpdateContainerInstancesState(instanceARN, status)
	assert.NoError(t, err, fmt.Sprintf("Unexpected error calling UpdateContainerInstancesState: %s", err))
}

func TestUpdateContainerInstancesStateError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	instanceARN := "myInstanceARN"
	status := "DRAINING"
	mc.EXPECT().UpdateContainerInstancesState(&ecs.UpdateContainerInstancesStateInput{
		ContainerInstances: []*string{aws.String(instanceARN)},
		Status:             aws.String(status),
		Cluster:            aws.String(configuredCluster),
	}).Return(nil, fmt.Errorf("ERROR"))

	err := client.UpdateContainerInstancesState(instanceARN, status)
	assert.Error(t, err, "Expected an error calling UpdateContainerInstancesState but got nil")
}

func TestGetResourceTags(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	instanceARN := "myInstanceARN"
	mc.EXPECT().ListTagsForResource(&ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(instanceARN),
	}).Return(&ecs.ListTagsForResourceOutput{
		Tags: containerInstanceTags,
	}, nil)

	_, err := client.GetResourceTags(instanceARN)
	assert.NoError(t, err, fmt.Sprintf("Unexpected error calling GetResourceTags: %s", err))
}

func TestGetResourceTagsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)

	instanceARN := "myInstanceARN"
	mc.EXPECT().ListTagsForResource(&ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(instanceARN),
	}).Return(nil, fmt.Errorf("ERROR"))

	_, err := client.GetResourceTags(instanceARN)
	assert.Error(t, err, "Expected an error calling GetResourceTags but got nil")
}

func TestDiscoverPollEndpointCacheHit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	pollEndpointCache := mock_async.NewMockTTLCache(mockCtrl)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:    mockSDK,
		ec2metadata:       ec2.NewBlackholeEC2MetadataClient(),
		pollEndpointCache: pollEndpointCache,
	}

	pollEndpoint := "http://127.0.0.1"
	pollEndpointCache.EXPECT().Get("containerInstance").Return(
		&ecs.DiscoverPollEndpointOutput{
			Endpoint: aws.String(pollEndpoint),
		}, false, true)
	output, err := client.discoverPollEndpoint("containerInstance")
	if err != nil {
		t.Fatalf("Error in discoverPollEndpoint: %v", err)
	}
	if aws.StringValue(output.Endpoint) != pollEndpoint {
		t.Errorf("Mismatch in poll endpoint: %s != %s", aws.StringValue(output.Endpoint), pollEndpoint)
	}
}

func TestDiscoverPollEndpointCacheMiss(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	pollEndpointCache := mock_async.NewMockTTLCache(mockCtrl)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:    mockSDK,
		ec2metadata:       ec2.NewBlackholeEC2MetadataClient(),
		pollEndpointCache: pollEndpointCache,
	}
	pollEndpoint := "http://127.0.0.1"
	pollEndpointOutput := &ecs.DiscoverPollEndpointOutput{
		Endpoint: &pollEndpoint,
	}

	gomock.InOrder(
		pollEndpointCache.EXPECT().Get("containerInstance").Return(nil, false, false),
		mockSDK.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(pollEndpointOutput, nil),
		pollEndpointCache.EXPECT().Set("containerInstance", pollEndpointOutput),
	)

	output, err := client.discoverPollEndpoint("containerInstance")
	if err != nil {
		t.Fatalf("Error in discoverPollEndpoint: %v", err)
	}
	if aws.StringValue(output.Endpoint) != pollEndpoint {
		t.Errorf("Mismatch in poll endpoint: %s != %s", aws.StringValue(output.Endpoint), pollEndpoint)
	}
}

func TestDiscoverPollEndpointExpiredButDPEFailed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	pollEndpointCache := mock_async.NewMockTTLCache(mockCtrl)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:    mockSDK,
		ec2metadata:       ec2.NewBlackholeEC2MetadataClient(),
		pollEndpointCache: pollEndpointCache,
	}
	pollEndpoint := "http://127.0.0.1"
	pollEndpointOutput := &ecs.DiscoverPollEndpointOutput{
		Endpoint: &pollEndpoint,
	}

	gomock.InOrder(
		pollEndpointCache.EXPECT().Get("containerInstance").Return(pollEndpointOutput, true, false),
		mockSDK.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(nil, fmt.Errorf("error!")),
	)

	output, err := client.discoverPollEndpoint("containerInstance")
	if err != nil {
		t.Fatalf("Error in discoverPollEndpoint: %v", err)
	}
	if aws.StringValue(output.Endpoint) != pollEndpoint {
		t.Errorf("Mismatch in poll endpoint: %s != %s", aws.StringValue(output.Endpoint), pollEndpoint)
	}
}

func TestDiscoverTelemetryEndpointAfterPollEndpointCacheHit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	pollEndpointCache := async.NewTTLCache(10 * time.Minute)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:    mockSDK,
		ec2metadata:       ec2.NewBlackholeEC2MetadataClient(),
		pollEndpointCache: pollEndpointCache,
	}

	pollEndpoint := "http://127.0.0.1"
	mockSDK.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(
		&ecs.DiscoverPollEndpointOutput{
			Endpoint:          &pollEndpoint,
			TelemetryEndpoint: &pollEndpoint,
		}, nil)
	endpoint, err := client.DiscoverPollEndpoint("containerInstance")
	if err != nil {
		t.Fatalf("Error in discoverPollEndpoint: %v", err)
	}
	if endpoint != pollEndpoint {
		t.Errorf("Mismatch in poll endpoint: %s", endpoint)
	}
	telemetryEndpoint, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err != nil {
		t.Fatalf("Error in discoverTelemetryEndpoint: %v", err)
	}
	if telemetryEndpoint != pollEndpoint {
		t.Errorf("Mismatch in poll endpoint: %s", endpoint)
	}
}

// TestSubmitTaskStateChangeWithAttachments tests the SubmitTaskStateChange API
// also send the Attachment Status
func TestSubmitTaskStateChangeWithAttachments(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mockSubmitStateClient.EXPECT().SubmitTaskStateChange(&taskSubmitInputMatcher{
		ecs.SubmitTaskStateChangeInput{
			Cluster: aws.String(configuredCluster),
			Task:    aws.String("task_arn"),
			Attachments: []*ecs.AttachmentStateChange{
				{
					AttachmentArn: aws.String("eni_arn"),
					Status:        aws.String("ATTACHED"),
				},
			},
		},
	})

	err := client.SubmitTaskStateChange(api.TaskStateChange{
		TaskARN: "task_arn",
		Attachment: &apieni.ENIAttachment{
			AttachmentARN: "eni_arn",
			Status:        apieni.ENIAttached,
		},
	})
	assert.NoError(t, err, "Unable to submit task state change with attachments")
}

func TestSubmitTaskStateChangeWithoutAttachments(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mockSubmitStateClient.EXPECT().SubmitTaskStateChange(&taskSubmitInputMatcher{
		ecs.SubmitTaskStateChangeInput{
			Cluster: aws.String(configuredCluster),
			Task:    aws.String("task_arn"),
			Reason:  aws.String(""),
			Status:  aws.String("RUNNING"),
		},
	})

	err := client.SubmitTaskStateChange(api.TaskStateChange{
		TaskARN: "task_arn",
		Status:  apitaskstatus.TaskRunning,
	})
	assert.NoError(t, err, "Unable to submit task state change with no attachments")
}

func TestSubmitTaskStateChangeWithManagedAgents(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
	mockSubmitStateClient.EXPECT().SubmitTaskStateChange(&taskSubmitInputMatcher{
		ecs.SubmitTaskStateChangeInput{
			Cluster:       aws.String(configuredCluster),
			Task:          aws.String("task_arn"),
			Reason:        aws.String(""),
			Status:        aws.String("RUNNING"),
			ManagedAgents: testManagedAgents,
		},
	})

	err := client.SubmitTaskStateChange(api.TaskStateChange{
		TaskARN: "task_arn",
		Status:  apitaskstatus.TaskRunning,
	})
	assert.NoError(t, err, "Unable to submit task state change with managed agents")
}

// TestSubmitContainerStateChangeWhileTaskInPending tests the container state change was submitted
// when the task is still in pending state
func TestSubmitContainerStateChangeWhileTaskInPending(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

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

	taskStateChangePending := api.TaskStateChange{
		Status:  apitaskstatus.TaskCreated,
		TaskARN: "arn",
		Containers: []api.ContainerStateChange{
			{
				TaskArn:       "arn",
				ContainerName: "container",
				RuntimeID:     "runtimeid",
				Status:        apicontainerstatus.ContainerRunning,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("TaskStatus: %s", tc.taskStatus.String()), func(t *testing.T) {
			taskStateChangePending.Status = tc.taskStatus
			client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient(), nil)
			mockSubmitStateClient.EXPECT().SubmitTaskStateChange(&taskSubmitInputMatcher{
				ecs.SubmitTaskStateChangeInput{
					Cluster: strptr(configuredCluster),
					Task:    strptr("arn"),
					Status:  strptr("PENDING"),
					Reason:  strptr(""),
					Containers: []*ecs.ContainerStateChange{
						{
							ContainerName:   strptr("container"),
							RuntimeId:       strptr("runtimeid"),
							Status:          strptr("RUNNING"),
							NetworkBindings: []*ecs.NetworkBinding{},
						},
					},
				},
			})
			err := client.SubmitTaskStateChange(taskStateChangePending)
			assert.NoError(t, err)
		})
	}
}

func extractTagsMapFromRegisterContainerInstanceInput(req *ecs.RegisterContainerInstanceInput) map[string]string {
	tagsMap := make(map[string]string, len(req.Tags))
	for i := range req.Tags {
		tagsMap[aws.StringValue(req.Tags[i].Key)] = aws.StringValue(req.Tags[i].Value)
	}
	return tagsMap
}
