// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/async/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const (
	configuredCluster = "mycluster"
	mac               = "01:23:45:67:89:ab"
	vpcID             = "vpc-1234"
	subnetID          = "subnet-1234"
	iid               = "instanceIdentityDocument"
	iidSignature      = "signature"
)

var (
	iidResponse          = []byte(iid)
	iidSignatureResponse = []byte(iidSignature)
)

func NewMockClient(ctrl *gomock.Controller, ec2Metadata ec2.EC2MetadataClient, additionalAttributes map[string]string) (api.ECSClient, *mock_api.MockECSSDK, *mock_api.MockECSSubmitStateSDK) {
	client := NewECSClient(credentials.AnonymousCredentials,
		&config.Config{Cluster: configuredCluster,
			AWSRegion:          "us-east-1",
			InstanceAttributes: additionalAttributes,
		}, ec2Metadata)
	mockSDK := mock_api.NewMockECSSDK(ctrl)
	mockSubmitStateSDK := mock_api.NewMockECSSubmitStateSDK(ctrl)
	client.(*APIECSClient).SetSDK(mockSDK)
	client.(*APIECSClient).SetSubmitStateChangeSDK(mockSubmitStateSDK)
	return client, mockSDK, mockSubmitStateSDK
}

type containerSubmitInputMatcher struct {
	ecs.SubmitContainerStateChangeInput
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
		equal(lhs.Reason, rhs.Reason) &&
		equal(lhs.Status, rhs.Status) &&
		equal(lhs.Task, rhs.Task))
}

func (lhs *containerSubmitInputMatcher) String() string {
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
			Status:        strptr("RUNNING"),
			NetworkBindings: []*ecs.NetworkBinding{
				&ecs.NetworkBinding{
					BindIP:        strptr("1.2.3.4"),
					ContainerPort: int64ptr(intptr(1)),
					HostPort:      int64ptr(intptr(2)),
					Protocol:      strptr("tcp"),
				},
				&ecs.NetworkBinding{
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
		Status:        api.ContainerRunning,
		PortBindings: []api.PortBinding{
			api.PortBinding{
				BindIP:        "1.2.3.4",
				ContainerPort: 1,
				HostPort:      2,
			},
			api.PortBinding{
				BindIP:        "2.2.3.4",
				ContainerPort: 3,
				HostPort:      4,
				Protocol:      api.TransportProtocolUDP,
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
			Status:        strptr("STOPPED"),
			ExitCode:      int64ptr(&exitCode),
			Reason:        strptr(reason),
			NetworkBindings: []*ecs.NetworkBinding{
				&ecs.NetworkBinding{
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
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
		PortBindings: []api.PortBinding{
			api.PortBinding{},
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
	reason := strings.Repeat("a", ecsMaxReasonLength)

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
		Status:        api.ContainerStopped,
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
	trimmedReason := strings.Repeat("a", ecsMaxReasonLength)
	reason := strings.Repeat("a", ecsMaxReasonLength+1)

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
		Status:        api.ContainerStopped,
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

func TestReRegisterContainerInstance(t *testing.T) {
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute":    "Custom_Value2",
		"attribute_name_with_no_value": "",
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata, additionalAttributes)

	capabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.vpc-id":    vpcID,
		"ecs.subnet-id": subnetID,
		"ecs.os-type":   api.OSType,
	}
	for i := range capabilities {
		expectedAttributes[capabilities[i]] = ""
	}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockEC2Metadata.EXPECT().SubnetID(mac).Return(subnetID, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentResource).Return(iidResponse, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentSignatureResource).Return(iidSignatureResponse, nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			assert.Equal(t, "arn:test", *req.ContainerInstanceArn, "Wrong container instance ARN")
			assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
			assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
			assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
			resource, ok := findResource(req.TotalResources, "PORTS_UDP")
			assert.True(t, ok, `Could not find resource "PORTS_UDP"`)
			assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
			assert.Equal(t, 5, len(req.Attributes), "Wrong number of Attributes")
			reqAttributes := func() map[string]string {
				rv := make(map[string]string, len(req.Attributes))
				for i := range req.Attributes {
					rv[*req.Attributes[i].Name] = aws.StringValue(req.Attributes[i].Value)
				}
				return rv
			}()
			for k, v := range reqAttributes {
				assert.Contains(t, expectedAttributes, k)
				assert.Equal(t, expectedAttributes[k], v)
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(capabilities, expectedAttributes),
			}},
			nil),
	)

	arn, err := client.RegisterContainerInstance("arn:test", capabilities)
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
}

func TestRegisterContainerInstance(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata, additionalAttributes)

	capabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.os-type":               api.OSType,
		"ecs.vpc-id":                vpcID,
		"ecs.subnet-id":             subnetID,
		"my_custom_attribute":       "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockEC2Metadata.EXPECT().SubnetID(mac).Return(subnetID, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentResource).Return(iidResponse, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentSignatureResource).Return(iidSignatureResponse, nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			assert.Nil(t, req.ContainerInstanceArn)
			assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
			assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
			assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
			resource, ok := findResource(req.TotalResources, "PORTS_UDP")
			assert.True(t, ok, `Could not find resource "PORTS_UDP"`)
			assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
			assert.Equal(t, 7, len(req.Attributes), "Wrong number of Attributes")
			for i := range req.Attributes {
				if strings.Contains(*req.Attributes[i].Name, "capability") {
					assert.Contains(t, capabilities, *req.Attributes[i].Name)
				} else {
					assert.Equal(t, expectedAttributes[*req.Attributes[i].Name], *req.Attributes[i].Value)
				}
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(capabilities, expectedAttributes)}},
			nil),
	)

	arn, err := client.RegisterContainerInstance("", capabilities)
	assert.NoError(t, err)
	assert.Equal(t, "registerArn", arn)
}

func TestRegisterContainerInstanceInClassicEC2(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata, additionalAttributes)

	capabilities := []string{"capability1", "capability2"}
	expectedAttributes := map[string]string{
		"ecs.os-type":               api.OSType,
		"my_custom_attribute":       "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}

	gomock.InOrder(
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, ec2.NewMetadataError(http.StatusNotFound)),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentResource).Return(iidResponse, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentSignatureResource).Return(iidSignatureResponse, nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			assert.Nil(t, req.ContainerInstanceArn)
			assert.Equal(t, configuredCluster, *req.Cluster, "Wrong cluster")
			assert.Equal(t, iid, *req.InstanceIdentityDocument, "Wrong IID")
			assert.Equal(t, iidSignature, *req.InstanceIdentityDocumentSignature, "Wrong IID sig")
			assert.Equal(t, 4, len(req.TotalResources), "Wrong length of TotalResources")
			resource, ok := findResource(req.TotalResources, "PORTS_UDP")
			assert.True(t, ok, `Could not find resource "PORTS_UDP"`)
			assert.Equal(t, "STRINGSET", *resource.Type, `Wrong type for resource "PORTS_UDP"`)
			assert.Equal(t, 5, len(req.Attributes), "Wrong number of Attributes")
			for i := range req.Attributes {
				if strings.Contains(*req.Attributes[i].Name, "capability") {
					assert.Contains(t, capabilities, *req.Attributes[i].Name)
				} else {
					assert.Equal(t, expectedAttributes[*req.Attributes[i].Name], *req.Attributes[i].Value)
				}
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{
			ContainerInstance: &ecs.ContainerInstance{
				ContainerInstanceArn: aws.String("registerArn"),
				Attributes:           buildAttributeList(capabilities, expectedAttributes)}},
			nil),
	)

	arn, err := client.RegisterContainerInstance("", capabilities)
	assert.NoError(t, err)
	assert.Equal(t, "registerArn", arn)
}

func TestRegisterContainerInstanceGetVPCIDError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	capabilities := []string{"capability1", "capability2"}
	additionalAttributes := map[string]string{"my_custom_attribute": "Custom_Value1",
		"my_other_custom_attribute": "Custom_Value2",
	}
	client, _, _ := NewMockClient(mockCtrl, mockEC2Metadata, additionalAttributes)

	gomock.InOrder(
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, errors.New("error")),
	)

	_, err := client.RegisterContainerInstance("", capabilities)
	assert.Error(t, err)
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
	client := NewECSClient(credentials.AnonymousCredentials, &config.Config{Cluster: "", AWSRegion: "us-east-1"}, mockEC2Metadata)
	mc := mock_api.NewMockECSSDK(mockCtrl)
	client.(*APIECSClient).SetSDK(mc)

	expectedAttributes := map[string]string{
		"ecs.os-type":   api.OSType,
		"ecs.vpc-id":    vpcID,
		"ecs.subnet-id": subnetID,
	}
	defaultCluster := config.DefaultClusterName
	gomock.InOrder(
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockEC2Metadata.EXPECT().SubnetID(mac).Return(subnetID, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentResource).Return(iidResponse, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentSignatureResource).Return(iidSignatureResponse, nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Return(nil, awserr.New("ClientException", "No such cluster", errors.New("No such cluster"))),
		mc.EXPECT().CreateCluster(&ecs.CreateClusterInput{ClusterName: &defaultCluster}).Return(&ecs.CreateClusterOutput{Cluster: &ecs.Cluster{ClusterName: &defaultCluster}}, nil),
		mockEC2Metadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockEC2Metadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockEC2Metadata.EXPECT().SubnetID(mac).Return(subnetID, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentResource).Return(iidResponse, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.InstanceIdentityDocumentSignatureResource).Return(iidSignatureResponse, nil),
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

	arn, err := client.RegisterContainerInstance("", nil)
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
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

func TestDiscoverPollEndpointCacheHit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSDK := mock_api.NewMockECSSDK(mockCtrl)
	pollEndpoinCache := mock_async.NewMockCache(mockCtrl)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:   mockSDK,
		ec2metadata:      ec2.NewBlackholeEC2MetadataClient(),
		pollEndpoinCache: pollEndpoinCache,
	}

	pollEndpoint := "http://127.0.0.1"
	pollEndpoinCache.EXPECT().Get("containerInstance").Return(
		&ecs.DiscoverPollEndpointOutput{
			Endpoint: aws.String(pollEndpoint),
		}, true)
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
	pollEndpoinCache := mock_async.NewMockCache(mockCtrl)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:   mockSDK,
		ec2metadata:      ec2.NewBlackholeEC2MetadataClient(),
		pollEndpoinCache: pollEndpoinCache,
	}
	pollEndpoint := "http://127.0.0.1"
	pollEndpointOutput := &ecs.DiscoverPollEndpointOutput{
		Endpoint: &pollEndpoint,
	}

	gomock.InOrder(
		pollEndpoinCache.EXPECT().Get("containerInstance").Return(nil, false),
		mockSDK.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(pollEndpointOutput, nil),
		pollEndpoinCache.EXPECT().Set("containerInstance", pollEndpointOutput),
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
	pollEndpoinCache := async.NewLRUCache(1, 10*time.Minute)
	client := &APIECSClient{
		credentialProvider: credentials.AnonymousCredentials,
		config: &config.Config{
			Cluster:   configuredCluster,
			AWSRegion: "us-east-1",
		},
		standardClient:   mockSDK,
		ec2metadata:      ec2.NewBlackholeEC2MetadataClient(),
		pollEndpoinCache: pollEndpoinCache,
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

func TestGetVPCAttributesHappyPath(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockMetadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client := &APIECSClient{
		ec2metadata: mockMetadata,
	}

	gomock.InOrder(
		mockMetadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockMetadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockMetadata.EXPECT().SubnetID(mac).Return(subnetID, nil),
	)

	expectedAttributes := []struct {
		name  string
		value string
	}{
		{"ecs.vpc-id", vpcID},
		{"ecs.subnet-id", subnetID},
	}
	attributes, err := client.getVPCAttributes()
	assert.NoError(t, err)
	assert.Len(t, attributes, 2)

	for i, attribute := range attributes {
		assert.Equal(t, expectedAttributes[i].name, aws.StringValue(attribute.Name))
		assert.Equal(t, expectedAttributes[i].value, aws.StringValue(attribute.Value))
	}
}

func TestGetVPCAttributesPrimaryENIMACError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockMetadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client := &APIECSClient{
		ec2metadata: mockMetadata,
	}

	mockMetadata.EXPECT().PrimaryENIMAC().Return("", errors.New("error"))

	attributes, err := client.getVPCAttributes()
	assert.Empty(t, attributes)
	assert.Error(t, err)
}

func TestGetVPCAttributesVPCIDError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockMetadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client := &APIECSClient{
		ec2metadata: mockMetadata,
	}

	gomock.InOrder(
		mockMetadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockMetadata.EXPECT().VPCID(mac).Return("", errors.New("error")),
	)

	attributes, err := client.getVPCAttributes()
	assert.Empty(t, attributes)
	assert.Error(t, err)
}

func TestVPCAttributesSubnetIDError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockMetadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client := &APIECSClient{
		ec2metadata: mockMetadata,
	}

	gomock.InOrder(
		mockMetadata.EXPECT().PrimaryENIMAC().Return(mac, nil),
		mockMetadata.EXPECT().VPCID(mac).Return(vpcID, nil),
		mockMetadata.EXPECT().SubnetID(mac).Return("", errors.New("error")),
	)

	attributes, err := client.getVPCAttributes()
	assert.Empty(t, attributes)
	assert.Error(t, err)
}
