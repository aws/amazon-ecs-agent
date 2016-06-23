// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

const configuredCluster = "mycluster"

func NewMockClient(ctrl *gomock.Controller, ec2Metadata ec2.EC2MetadataClient) (api.ECSClient, *mock_api.MockECSSDK, *mock_api.MockECSSubmitStateSDK) {
	client := api.NewECSClient(credentials.AnonymousCredentials, &config.Config{Cluster: configuredCluster, AWSRegion: "us-east-1"}, http.DefaultClient, ec2Metadata)
	mockSDK := mock_api.NewMockECSSDK(ctrl)
	mockSubmitStateSDK := mock_api.NewMockECSSubmitStateSDK(ctrl)
	client.(*api.ApiECSClient).SetSDK(mockSDK)
	client.(*api.ApiECSClient).SetSubmitStateChangeSDK(mockSubmitStateSDK)
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
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
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
				BindIp:        "1.2.3.4",
				ContainerPort: 1,
				HostPort:      2,
			},
			api.PortBinding{
				BindIp:        "2.2.3.4",
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
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
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
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
	exitCode := 20
	reason := strings.Repeat("a", api.EcsMaxReasonLength)

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
	client, _, mockSubmitStateClient := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
	exitCode := 20
	trimmedReason := strings.Repeat("a", api.EcsMaxReasonLength)
	reason := strings.Repeat("a", api.EcsMaxReasonLength+1)

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

func TestRegisterContainerInstance(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(mockCtrl)
	client, mc, _ := NewMockClient(mockCtrl, mockEC2Metadata)

	capabilities := []string{"capability1", "capability2"}

	mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return([]byte("instanceIdentityDocument"), nil)
	mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_SIGNATURE_RESOURCE).Return([]byte("signature"), nil)
	mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
		if *req.ContainerInstanceArn != "arn:test" {
			t.Errorf("Wrong container instance ARN: %v", *req.ContainerInstanceArn)
		}
		if *req.Cluster != configuredCluster {
			t.Errorf("Wrong cluster: %v", *req.Cluster)
		}
		if *req.InstanceIdentityDocument != "instanceIdentityDocument" {
			t.Errorf("Wrong IID: %v", *req.InstanceIdentityDocument)
		}
		if *req.InstanceIdentityDocumentSignature != "signature" {
			t.Errorf("Wrong IID sig: %v", *req.InstanceIdentityDocumentSignature)
		}
		if len(req.TotalResources) != 4 {
			t.Errorf("Wrong length of TotalResources, expected 4 but was %d", len(req.TotalResources))
		}
		resource, ok := findResource(req.TotalResources, "PORTS_UDP")
		if !ok {
			t.Error("Could not find resource \"PORTS_UDP\"")
		}
		if *resource.Type != "STRINGSET" {
			t.Errorf("Wrong type for resource \"PORTS_UDP\".  Expected \"STRINGSET\" but was \"%s\"", *resource.Type)
		}
		if len(req.Attributes) != len(capabilities) {
			t.Errorf("Wrong lenght of Attributes, expected %d but was %d", len(capabilities), len(req.Attributes))
		}
		for i, _ := range capabilities {
			if req.Attributes[i].Name == nil {
				t.Errorf("nil name for attribute %d", i)
			}
			if *req.Attributes[i].Name != capabilities[i] {
				t.Errorf("Wrong attribute, expected %s but was %s", capabilities[i], req.Attributes[i])
			}
		}

	}).Return(&ecs.RegisterContainerInstanceOutput{ContainerInstance: &ecs.ContainerInstance{ContainerInstanceArn: aws.String("registerArn")}}, nil)

	arn, err := client.RegisterContainerInstance("arn:test", capabilities)
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
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
	client := api.NewECSClient(credentials.AnonymousCredentials, &config.Config{Cluster: "", AWSRegion: "us-east-1"}, http.DefaultClient, mockEC2Metadata)
	mc := mock_api.NewMockECSSDK(mockCtrl)
	client.(*api.ApiECSClient).SetSDK(mc)

	defaultCluster := config.DefaultClusterName
	gomock.InOrder(
		mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return([]byte("instanceIdentityDocument"), nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_SIGNATURE_RESOURCE).Return([]byte("signature"), nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Return(nil, awserr.New("ClientException", "No such cluster", errors.New("No such cluster"))),
		mc.EXPECT().CreateCluster(&ecs.CreateClusterInput{ClusterName: &defaultCluster}).Return(&ecs.CreateClusterOutput{Cluster: &ecs.Cluster{ClusterName: &defaultCluster}}, nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return([]byte("instanceIdentityDocument"), nil),
		mockEC2Metadata.EXPECT().ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_SIGNATURE_RESOURCE).Return([]byte("signature"), nil),
		mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
			if *req.Cluster != config.DefaultClusterName {
				t.Errorf("Wrong cluster: %v", *req.Cluster)
			}
			if *req.InstanceIdentityDocument != "instanceIdentityDocument" {
				t.Errorf("Wrong IID: %v", *req.InstanceIdentityDocument)
			}
			if *req.InstanceIdentityDocumentSignature != "signature" {
				t.Errorf("Wrong IID sig: %v", *req.InstanceIdentityDocumentSignature)
			}
		}).Return(&ecs.RegisterContainerInstanceOutput{ContainerInstance: &ecs.ContainerInstance{ContainerInstanceArn: aws.String("registerArn")}}, nil),
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
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
	expectedEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{TelemetryEndpoint: &expectedEndpoint}, nil)
	endpoint, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err != nil {
		t.Error("Error getting telemetry endpoint: ", err)
	}
	if expectedEndpoint != endpoint {
		t.Errorf("Expected telemetry endpoint(%s) != endpoint(%s)", expectedEndpoint, endpoint)
	}
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(nil, fmt.Errorf("Error getting endpoint"))
	endpoint, err = client.DiscoverTelemetryEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting telemetry endpoint, didn't get any")
	}
}

func TestDiscoverNilTelemetryEndpoint(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc, _ := NewMockClient(mockCtrl, ec2.NewBlackholeEC2MetadataClient())
	pollEndpoint := "http://127.0.0.1"
	mc.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(&ecs.DiscoverPollEndpointOutput{Endpoint: &pollEndpoint}, nil)
	_, err := client.DiscoverTelemetryEndpoint("containerInstance")
	if err == nil {
		t.Error("Expected error getting telemetry endpoint with old response")
	}
}
