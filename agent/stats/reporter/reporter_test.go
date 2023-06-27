package reporter

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testContainerInstanceArn = "testContainerInstanceArn"
	testCluster              = "testCluster"
	testRegion               = "us-west-2"
	testDockerEndpoint       = "testDockerEndpoint"
	testDockerVersion        = "testDockerVersion"
)

func TestNewDockerTelemetrySession(t *testing.T) {
	emptyDoctor, _ := doctor.NewDoctor([]doctor.Healthcheck{}, testCluster, testContainerInstanceArn)
	testCredentials := credentials.NewStaticCredentials("test-id", "test-secret", "test-token")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEngine := mock_engine.NewMockTaskEngine(ctrl)
	mockEngine.EXPECT().Version().Return(testDockerVersion, nil)
	testCases := []struct {
		name            string
		cfg             *config.Config
		expectedSession bool
		expectedError   bool
	}{
		{
			name: "happy case",
			cfg: &config.Config{
				DisableMetrics:           config.BooleanDefaultFalse{},
				DisableDockerHealthCheck: config.BooleanDefaultFalse{},
				Cluster:                  testCluster,
				AWSRegion:                testRegion,
				AcceptInsecureCert:       false,
				DockerEndpoint:           testDockerEndpoint,
			},
			expectedSession: true,
			expectedError:   false,
		},
		{
			name:            "cfg parsing error",
			cfg:             nil,
			expectedSession: false,
			expectedError:   true,
		},
		{
			name: "metrics disabled",
			cfg: &config.Config{
				DisableMetrics: config.BooleanDefaultFalse{
					Value: config.ExplicitlyEnabled,
				},
				DisableDockerHealthCheck: config.BooleanDefaultFalse{
					Value: config.ExplicitlyEnabled,
				},
				Cluster:            testCluster,
				AWSRegion:          testRegion,
				AcceptInsecureCert: false,
				DockerEndpoint:     testDockerEndpoint,
			},
			expectedSession: false,
			expectedError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dockerTelemetrySession, err := NewDockerTelemetrySession(
				testContainerInstanceArn,
				testCredentials,
				tc.cfg,
				eventstream.NewEventStream("Deregister_Instance", context.Background()),
				nil,
				mockEngine,
				nil,
				nil,
				emptyDoctor,
			)
			if tc.expectedSession {
				assert.NotNil(t, dockerTelemetrySession)
			} else {
				assert.Nil(t, dockerTelemetrySession)
			}

			if tc.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateVersionInfo_GetVersionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEngine := mock_engine.NewMockTaskEngine(ctrl)
	mockEngine.EXPECT().Version().Times(1).Return("", errors.New("error"))
	agentVersion, agentHash, containerRuntimeVersion := generateVersionInfo(mockEngine)
	assert.Equal(t, version.Version, agentVersion)
	assert.Equal(t, version.GitShortHash, agentHash)
	assert.Equal(t, "", containerRuntimeVersion)
}

func TestGenerateVersionInfo_NoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEngine := mock_engine.NewMockTaskEngine(ctrl)
	mockEngine.EXPECT().Version().Times(1).Return(testDockerVersion, nil)
	agentVersion, agentHash, containerRuntimeVersion := generateVersionInfo(mockEngine)
	assert.Equal(t, version.Version, agentVersion)
	assert.Equal(t, version.GitShortHash, agentHash)
	assert.Equal(t, testDockerVersion, containerRuntimeVersion)
}
