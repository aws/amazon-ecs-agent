package api_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"code.google.com/p/gomock/gomock"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/ecs"
)

const configuredCluster = "mycluster"

func NewMockClient(ctrl *gomock.Controller) (api.ECSClient, *mock_api.MockECSSDK) {
	client := api.NewECSClient(aws.DetectCreds("", "", ""), &config.Config{Cluster: configuredCluster, AWSRegion: "us-east-1"}, false)
	mock := mock_api.NewMockECSSDK(ctrl)
	client.(*api.ApiECSClient).SetSDK(mock)
	return client, mock
}

type containerSubmitInputMatcher struct {
	Cluster       string
	Arn           string
	ContainerName string
	Status        api.ContainerStatus
	ExitCode      *int
	Reason        *string
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

func (lhs *containerSubmitInputMatcher) Matches(x interface{}) bool {
	rhs := x.(*ecs.SubmitContainerStateChangeInput)
	if !(lhs.Arn == *rhs.Task && lhs.ContainerName == *rhs.ContainerName && lhs.Status.String() == *rhs.Status && lhs.Cluster == *rhs.Cluster) {
		return false
	}
	if !reflect.DeepEqual(lhs.Reason, rhs.Reason) || !reflect.DeepEqual(int64ptr(lhs.ExitCode), rhs.ExitCode) {
		return false
	}
	return true
}
func (lhs *containerSubmitInputMatcher) String() string {
	return fmt.Sprintf("%+v", *lhs)
}

func TestSubmitContainerStateChange(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	mc.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		Cluster:       configuredCluster,
		Arn:           "arn",
		ContainerName: "cont",
		Status:        api.ContainerRunning,
	})
	client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        api.ContainerRunning,
	})
}

func TestSubmitContainerStateChangeFull(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	exitCode := 20
	reason := "I exited"

	mc.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		Cluster:       configuredCluster,
		Arn:           "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        &reason,
	})
	client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
	})
}

func TestSubmitContainerStateChangeReason(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	exitCode := 20
	reason := strings.Repeat("a", api.EcsMaxReasonLength)

	mc.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		Cluster:       configuredCluster,
		Arn:           "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        &reason,
	})
	client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
	})
}

func TestSubmitContainerStateChangeLongReason(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	exitCode := 20
	trimmedReason := strings.Repeat("a", api.EcsMaxReasonLength)
	reason := strings.Repeat("a", api.EcsMaxReasonLength+1)

	mc.EXPECT().SubmitContainerStateChange(&containerSubmitInputMatcher{
		Cluster:       configuredCluster,
		Arn:           "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        &trimmedReason,
	})
	client.SubmitContainerStateChange(api.ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        api.ContainerStopped,
		ExitCode:      &exitCode,
		Reason:        reason,
	})
}

func TestRegisterContainerInstance(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
		if *req.Cluster != configuredCluster {
			t.Errorf("Wrong cluster: %v", *req.Cluster)
		}
		// TODO, IID, resources
	}).Return(&ecs.RegisterContainerInstanceOutput{ContainerInstance: &ecs.ContainerInstance{ContainerInstanceARN: aws.String("registerArn")}}, nil)

	arn, err := client.RegisterContainerInstance()
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
}

func TestRegisterBlankCluster(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client, mc := NewMockClient(mockCtrl)
	mc.EXPECT().RegisterContainerInstance(gomock.Any()).Do(func(req *ecs.RegisterContainerInstanceInput) {
		if *req.Cluster != configuredCluster {
			t.Errorf("Wrong cluster: %v", *req.Cluster)
		}
		// TODO, IID, resources
	}).Return(&ecs.RegisterContainerInstanceOutput{ContainerInstance: &ecs.ContainerInstance{ContainerInstanceARN: aws.String("registerArn")}}, nil)

	arn, err := client.RegisterContainerInstance()
	if err != nil {
		t.Errorf("Should not be an error: %v", err)
	}
	if arn != "registerArn" {
		t.Errorf("Wrong arn: %v", arn)
	}
}

// TODO, re-add RegisterContainerInstance tests after the few pending changes to
// defaultCluster are written
