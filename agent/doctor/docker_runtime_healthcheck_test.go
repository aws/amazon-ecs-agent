//go:build unit
// +build unit

package doctor

import (
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewDockerRuntimeHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerRuntimeHealthCheck := NewDockerRuntimeHealthcheck(mockDockerClient)
	assert.Equal(t, doctor.HealthcheckStatusInitializing, dockerRuntimeHealthCheck.Status)
}

func TestRunCheck(t *testing.T) {
	testcases := []struct {
		name               string
		dockerPingResponse *dockerapi.PingResponse
		expectedStatus     doctor.HealthcheckStatus
		expectedLastStatus doctor.HealthcheckStatus
	}{
		{
			name: "empty checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: &types.Ping{APIVersion: "test_api_version"},
				Error:    nil,
			},
			expectedStatus:     doctor.HealthcheckStatusOk,
			expectedLastStatus: doctor.HealthcheckStatusInitializing,
		},
		{
			name: "all true checks",
			dockerPingResponse: &dockerapi.PingResponse{
				Response: nil,
				Error:    &dockerapi.DockerTimeoutError{},
			},
			expectedStatus:     doctor.HealthcheckStatusImpaired,
			expectedLastStatus: doctor.HealthcheckStatusInitializing,
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
	healthCheckStatus := doctor.HealthcheckStatusOk
	dockerRuntimeHealthCheck.SetHealthcheckStatus(healthCheckStatus)
	assert.Equal(t, doctor.HealthcheckStatusOk, dockerRuntimeHealthCheck.Status)
}

func TestSetHealthcheckStatusChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerRuntimeHealthcheck := NewDockerRuntimeHealthcheck(dockerClient)

	// we should start in initializing status
	assert.Equal(t, doctor.HealthcheckStatusInitializing, dockerRuntimeHealthcheck.Status)
	initializationChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()

	// we update to initializing again; our StatusChangeTime remains the same
	dockerRuntimeHealthcheck.SetHealthcheckStatus(doctor.HealthcheckStatusInitializing)
	updateChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()
	assert.Equal(t, doctor.HealthcheckStatusInitializing, dockerRuntimeHealthcheck.Status)
	assert.Equal(t, initializationChangeTime, updateChangeTime)

	// add a sleep so we know time has elapsed between the initial status and status change time
	time.Sleep(1 * time.Millisecond)

	// change status.  This should change the update time too
	dockerRuntimeHealthcheck.SetHealthcheckStatus(doctor.HealthcheckStatusOk)
	assert.Equal(t, doctor.HealthcheckStatusOk, dockerRuntimeHealthcheck.Status)
	okChangeTime := dockerRuntimeHealthcheck.GetStatusChangeTime()
	// have we updated our change time?
	assert.True(t, okChangeTime.After(initializationChangeTime))
}
