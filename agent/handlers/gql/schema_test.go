package gql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/graphql-go/graphql"
	"github.com/stretchr/testify/assert"
)

const (
	clusterName              = "default"
	taskARN                  = "t1"
	cluster                  = "default"
	family                   = "sleep"
	version                  = "1"
	containerID              = "cid"
	containerName            = "sleepy"
	imageName                = "busybox"
	imageID                  = "bUsYbOx"
	containerType            = "NORMAL"
	cpu                      = 1024
	memory                   = 512
	statusRunning            = "RUNNING"
	eniIPv4Address           = "192.168.0.5"
	ipv4SubnetCIDRBlock      = "192.168.0.0/24"
	eniIPv6Address           = "2600:1f18:619e:f900:8467:78b2:81c4:207d"
	ipv6SubnetCIDRBlock      = "2600:1f18:619e:f900::/64"
	subnetGatewayIPV4Address = "192.168.0.1/24"
	volName                  = "volume1"
	volSource                = "/var/lib/volume1"
	volDestination           = "/volume"
	availabilityZone         = "us-west-2b"
	containerInstanceArn     = "containerInstance-test"
)

type gqlTask struct {
	*v4.TaskResponse
}
type gqlContainer struct {
	*v4.ContainerResponse
}

type gqlData struct {
	Container *gqlContainer `json:"Container,omitempty"`
	Task      *gqlTask      `json:"Task,omitempty"`
}

type gqlResponse struct {
	Data   *gqlData `json:"data"`
	Errors string   `json:"errors,omitempty"`
}

var (
	labels = map[string]string{
		"foo": "bar",
	}
	now             = time.Now().UTC()
	exitCode        = 0
	containerFields = v4.ContainerResponse{
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
			CreatedAt:  &now,
			StartedAt:  &now,
			FinishedAt: &now,
			ExitCode:   &exitCode,
			Type:       containerType,
			LogDriver:  "",
			LogOptions: map[string]string{},
			Labels:     labels,
		},
	}
	expectedContainerResponse = gqlResponse{
		Data: &gqlData{
			Container: &gqlContainer{
				&containerFields,
			},
		},
	}
	expectedTaskResponse = gqlResponse{
		Data: &gqlData{
			Task: &gqlTask{
				&v4.TaskResponse{
					TaskResponse: &v2.TaskResponse{
						Cluster:       clusterName,
						TaskARN:       taskARN,
						Family:        family,
						Revision:      version,
						DesiredStatus: statusRunning,
						KnownStatus:   statusRunning,
						Limits: &v2.LimitsResponse{
							CPU:    aws.Float64(cpu),
							Memory: aws.Int64(memory),
						},
						PullStartedAt:      aws.Time(now.UTC()),
						PullStoppedAt:      aws.Time(now.UTC()),
						ExecutionStoppedAt: aws.Time(now.UTC()),
						AvailabilityZone:   availabilityZone,
						LaunchType:         "EC2",
					},
					Containers: []v4.ContainerResponse{containerFields},
				},
			},
		},
	}
	expectedFailedContainerTaskResponse = gqlResponse{
		Errors: "Unable to get container name mapping for task " + taskARN,
	}
	expectedFailedContainerCTXResponse = gqlResponse{
		Errors: "Could not cast to container",
	}
	expectedFailedTaskCTXResponse = gqlResponse{
		Errors: "Could not cast to task",
	}
	containerQueryGQL = "{Container{DockerId,Name,DockerName,KnownStatus,DesiredStatus,CreatedAt,StartedAt,FinishedAt,ExitCode,Image,ImageID,Labels,Type,LogDriver,LogOptions,Limits,ContainerARN}}"
	taskQueryGQL      = "{Task{Cluster,TaskARN,Family,Revision,DesiredStatus,KnownStatus,Limits,PullStartedAt,PullStoppedAt,ExecutionStoppedAt,AvailabilityZone,LaunchType,Containers{DockerId,Name,DockerName,KnownStatus,DesiredStatus,CreatedAt,StartedAt,FinishedAt,ExitCode,Image,ImageID,Labels,Type,LogDriver,LogOptions,Limits,ContainerARN}}}"
)

func TestCreateSchema(t *testing.T) {
	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
					{
						Address: eniIPv4Address,
					},
				},
				IPV6Addresses: []*apieni.ENIIPV6Address{
					{
						Address: eniIPv6Address,
					},
				},
				SubnetGatewayIPV4Address: subnetGatewayIPV4Address,
			},
		},
		CPU:                      cpu,
		Memory:                   memory,
		PullStartedAtUnsafe:      now,
		PullStoppedAtUnsafe:      now,
		ExecutionStoppedAtUnsafe: now,
		LaunchType:               "EC2",
	}
	container := &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		ContainerArn:        "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
		VolumesUnsafe: []types.MountPoint{
			{
				Name:        volName,
				Source:      volSource,
				Destination: volDestination,
			},
		},
	}
	container.SetCreatedAt(now)
	container.SetStartedAt(now)
	container.SetFinishedAt(now)
	container.SetKnownExitCode(&exitCode)
	labels := map[string]string{
		"foo": "bar",
	}
	container.SetLabels(labels)
	dockerContainer := &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}
	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}

	testCases := []struct {
		name               string
		query              string
		taskContainerError bool
		containerCTXError  bool
		taskCTXError       bool
		expectedResponse   gqlResponse
	}{
		{
			name:               "Conainer Query",
			query:              containerQueryGQL,
			taskContainerError: false,
			containerCTXError:  false,
			taskCTXError:       false,
			expectedResponse:   expectedContainerResponse,
		},
		{
			name:               "Task Query",
			query:              taskQueryGQL,
			taskContainerError: false,
			containerCTXError:  false,
			taskCTXError:       false,
			expectedResponse:   expectedTaskResponse,
		},
		{
			name:               "No Container Task Query",
			query:              taskQueryGQL,
			taskContainerError: true,
			containerCTXError:  false,
			taskCTXError:       false,
			expectedResponse:   expectedFailedContainerTaskResponse,
		},
		{
			name:               "Get Container ctx Error",
			query:              containerQueryGQL,
			taskContainerError: false,
			containerCTXError:  true,
			taskCTXError:       false,
			expectedResponse:   expectedFailedContainerCTXResponse,
		},
		{
			name:               "Get Task ctx Error",
			query:              taskQueryGQL,
			taskContainerError: false,
			containerCTXError:  false,
			taskCTXError:       true,
			expectedResponse:   expectedFailedTaskCTXResponse,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			ecsClient := mock_api.NewMockECSClient(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)

			schema, err := CreateSchema(state, ecsClient, statsEngine, cluster, availabilityZone, containerInstanceArn)
			assert.NoError(t, err)
			var ctx context.Context
			if tc.containerCTXError {
				ctx = context.WithValue(context.Background(), Container, "container")
			} else if tc.taskCTXError {
				ctx = context.WithValue(context.Background(), Task, "task")
			} else {
				ctx = context.WithValue(context.Background(), Container, dockerContainer)
				ctx = context.WithValue(ctx, Task, task)
			}

			if tc.taskContainerError {
				state.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)
			} else {
				state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true).AnyTimes()
			}
			params := graphql.Params{
				Schema:        schema,
				RequestString: tc.query,
				Context:       ctx,
			}
			res := graphql.Do(params)
			if tc.taskContainerError {
				assert.Equal(t, tc.expectedResponse.Errors, res.Errors[0].Message)
				return
			}
			if tc.containerCTXError {
				assert.Equal(t, tc.expectedResponse.Errors, res.Errors[0].Message)
				return
			}
			if tc.taskCTXError {
				assert.Equal(t, tc.expectedResponse.Errors, res.Errors[0].Message)
				return
			}

			// Marshal GraphQL response then unmarshal into the expected response struct
			// in order to test whether the response fields are correct
			var response gqlResponse
			result, err := json.Marshal(res)
			assert.NoError(t, err)
			err = json.Unmarshal(result, &response)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedResponse, response)
		})
	}
}
