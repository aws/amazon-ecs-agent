//go:build unit
// +build unit

package doctor

import (
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewDockerRuntimeHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	// Mock the time to have predictable values
	mockTime := time.Now()
	originalTimeNow := timeNow
	timeNow = func() time.Time { return mockTime }
	defer func() { timeNow = originalTimeNow }()

	expectedDockerRuntimeHealthcheck := &dockerRuntimeHealthcheck{
		HealthcheckType:  ecstcs.InstanceHealthCheckTypeContainerRuntime,
		Status:           ecstcs.InstanceHealthCheckStatusInitializing,
		TimeStamp:        mockTime,
		StatusChangeTime: mockTime,
		LastTimeStamp:    mockTime,
		client:           mockDockerClient,
	}
	actualDockerRuntimeHealthcheck := NewDockerRuntimeHealthcheck(mockDockerClient)
	assert.Equal(t, expectedDockerRuntimeHealthcheck, actualDockerRuntimeHealthcheck)
}

func TestRunCheck(t *testing.T) {
	testcases := []struct {
		name               string
		dockerPingResponse *dockerapi.PingResponse
		expectedStatus     ecstcs.InstanceHealthCheckStatus
		expectedLastStatus ecstcs.InstanceHealthCheckStatus
	}{
		{
			name: "empty checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: &types.Ping{APIVersion: "test_api_version"},
				Error:    nil,
			},
			expectedStatus:     ecstcs.InstanceHealthCheckStatusOk,
			expectedLastStatus: ecstcs.InstanceHealthCheckStatusInitializing,
		},
		{
			name: "all true checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: nil,
				Error:    &dockerapi.DockerTimeoutError{},
			},
			expectedStatus:     ecstcs.InstanceHealthCheckStatusImpaired,
			expectedLastStatus: ecstcs.InstanceHealthCheckStatusInitializing,
		},
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			dockerRuntimeHealthCheck := NewDockerRuntimeHealthcheck(dockerClient)
			dockerClient.EXPECT().SystemPing(gomock.Any(), gomock.Any()).Return(*tc.dockerPingResponse)
			dockerRuntimeHealthCheck.RunCheck()
			assert.Equal(t, tc.expectedStatus, dockerRuntimeHealthCheck.Status)
			assert.Equal(t, tc.expectedLastStatus, dockerRuntimeHealthCheck.LastStatus)

		})
	}
}

func TestSetHealthCheckStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerRuntimeHealthCheck := NewDockerRuntimeHealthcheck(dockerClient)
	healthCheckStatus := ecstcs.InstanceHealthCheckStatusOk
	dockerRuntimeHealthCheck.SetHealthcheckStatus(healthCheckStatus)
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, dockerRuntimeHealthCheck.Status)
}

func TestSetHealthcheckStatusChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerRuntimeHealthcheck := NewDockerRuntimeHealthcheck(dockerClient)

	// We should start in initializing status.
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusInitializing, dockerRuntimeHealthcheck.Status)
	initializationChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()

	// We update to initializing again; our StatusChangeTime remains the same.
	dockerRuntimeHealthcheck.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusInitializing)
	updateChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusInitializing, dockerRuntimeHealthcheck.Status)
	assert.Equal(t, initializationChangeTime, updateChangeTime)

	// Add a sleep so we know time has elapsed between the initial status and status change time.
	time.Sleep(1 * time.Millisecond)

	// Change status. This should change the update time too.
	dockerRuntimeHealthcheck.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusOk)
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, dockerRuntimeHealthcheck.Status)
	okChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()
	// Have we updated our change time?
	assert.True(t, okChangeTime.After(initializationChangeTime))
}
