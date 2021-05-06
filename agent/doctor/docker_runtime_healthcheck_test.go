package doctor

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewDockerRuntimeHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerRuntimeHealthCheck := NewDockerRuntimeHealthcheck(mockDockerClient)
	assert.Equal(t, HealthcheckStatusInitializing, dockerRuntimeHealthCheck.Status)
}

func TestRunCheck(t *testing.T) {
	testcases := []struct {
		name               string
		dockerPingResponse *dockerapi.PingResponse
		expectedStatus     HealthcheckStatus
		expectedLastStatus HealthcheckStatus
	}{
		{
			name: "empty checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: &types.Ping{APIVersion: "test_api_version"},
				Error:    nil,
			},
			expectedStatus:     HealthcheckStatusOk,
			expectedLastStatus: HealthcheckStatusInitializing,
		},
		{
			name: "all true checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: nil,
				Error:    &dockerapi.DockerTimeoutError{},
			},
			expectedStatus:     HealthcheckStatusImpaired,
			expectedLastStatus: HealthcheckStatusInitializing,
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
	healthCheckStatus := HealthcheckStatusOk
	dockerRuntimeHealthCheck.SetHealthcheckStatus(healthCheckStatus)
	assert.Equal(t, HealthcheckStatusOk, dockerRuntimeHealthCheck.Status)
}
