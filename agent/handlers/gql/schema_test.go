package gql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/pkg/errors"

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
	pulledContainerName      = "pulled"
	imageName                = "busybox"
	imageID                  = "bUsYbOx"
	containerType            = "NORMAL"
	cpu                      = 1024
	memory                   = 512
	statusRunning            = "RUNNING"
	statusPulled             = "PULLED"
	eniIPv4Address           = "192.168.0.5"
	ipv4SubnetCIDRBlock      = "192.168.0.0/24"
	eniIPv6Address           = "2600:1f18:619e:f900:8467:78b2:81c4:207d"
	ipv6SubnetCIDRBlock      = "2600:1f18:619e:f900::/64"
	attachmentIndex          = 0
	macAddress               = "06:96:9a:ce:a6:ce"
	volName                  = "volume1"
	volSource                = "/var/lib/volume1"
	volDestination           = "/volume"
	availabilityZone         = "us-west-2b"
	containerInstanceArn     = "containerInstance-test"
	privateDNSName           = "ip-172-31-47-69.us-west-2.compute.internal"
	subnetGatewayIPV4Address = "192.168.0.1/24"
)

type gqlTask struct {
	*v4.TaskResponse
}
type gqlContainer struct {
	*v4.ContainerResponse
	ContainerStats *types.StatsJSON `json:"Stats,omitempty"`
}

type gqlData struct {
	Container *gqlContainer `json:"Container,omitempty"`
	Task      *gqlTask      `json:"Task,omitempty"`
}

type gqlResponse struct {
	Data   *gqlData `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

var (
	labels = map[string]string{
		"foo": "bar",
	}
	now                = time.Now().UTC()
	exitCode           = 0
	attachmentIndexVar = attachmentIndex
	dockerStats        = &types.StatsJSON{}
	containerFields    = v4.ContainerResponse{
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
		Networks: []v4.Network{{
			Network: containermetadata.Network{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: []string{eniIPv4Address},
				IPv6Addresses: []string{eniIPv6Address},
			},
			NetworkInterfaceProperties: v4.NetworkInterfaceProperties{
				AttachmentIndex:          &attachmentIndexVar,
				IPV4SubnetCIDRBlock:      ipv4SubnetCIDRBlock,
				IPv6SubnetCIDRBlock:      ipv6SubnetCIDRBlock,
				MACAddress:               macAddress,
				PrivateDNSName:           privateDNSName,
				SubnetGatewayIPV4Address: subnetGatewayIPV4Address,
			}},
		},
	}
	pulledContainerFields = v4.ContainerResponse{
		ContainerResponse: &v2.ContainerResponse{
			Name:          pulledContainerName,
			Image:         imageName,
			ImageID:       imageID,
			DesiredStatus: statusRunning,
			KnownStatus:   statusPulled,
			ContainerARN:  "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
			Limits: v2.LimitsResponse{
				CPU:    aws.Float64(cpu),
				Memory: aws.Int64(memory),
			},
			Type: containerType,
			//TODO: Find why default LogOptions is not empty map
			LogOptions: map[string]string{},
		},
	}
	expectedContainerResponse = gqlResponse{
		Data: &gqlData{
			Container: &gqlContainer{
				ContainerResponse: &containerFields,
			},
		},
	}
	expectedContainerStatsResponse = gqlResponse{
		Data: &gqlData{
			Container: &gqlContainer{
				ContainerStats: dockerStats,
			},
		},
	}
	expectedTagsResponse = gqlResponse{
		Data: &gqlData{
			Task: &gqlTask{
				TaskResponse: &v4.TaskResponse{
					TaskResponse: &v2.TaskResponse{
						ContainerInstanceTags: map[string]string{
							"ContainerInstanceTag1": "firstTag",
							"ContainerInstanceTag2": "secondTag",
						},
						TaskTags: map[string]string{
							"TaskTag1": "firstTag",
							"TaskTag2": "secondTag",
						},
					},
				},
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
					Containers: []v4.ContainerResponse{containerFields, pulledContainerFields},
				},
			},
		},
	}
	expectedFailedContainerTaskResponse = gqlResponse{
		Errors: []string{"Unable to get container name mapping for task " + taskARN},
	}
	expectedFailedContainerCTXResponse = gqlResponse{
		Errors: []string{"Could not cast to container"},
	}
	expectedFailedTaskCTXResponse = gqlResponse{
		Errors: []string{"Could not cast to task"},
	}
	expectedFailedTaskByIDResponse = gqlResponse{
		Errors: []string{"Unable to find task for container '" + containerID + "'"},
	}
	expectedFailedStatsResponse = gqlResponse{
		Errors: []string{"stats engine: task '" + taskARN + "' for container '" + containerID + "' not found"},
	}
	expectedFailedTagsResponse = gqlResponse{
		Errors: []string{"Task Metadata error: unable to get ContainerInstanceTags for '" + containerInstanceArn + "': Mock Error",
			"Task Metadata error: unable to get TaskTags for '" + taskARN + "': Mock Error"},
	}
	expectedFailedPulledContainerResponse = gqlResponse{
		Errors: []string{"Unable to get container name mapping for task" + taskARN},
	}
	containerQueryGQL         = "{Container{DockerId,Name,DockerName,KnownStatus,DesiredStatus,CreatedAt,StartedAt,FinishedAt,ExitCode,Image,ImageID,Labels,Type,LogDriver,LogOptions,Limits,ContainerARN,Networks}}"
	taskQueryGQL              = "{Task{Cluster,TaskARN,Family,Revision,DesiredStatus,KnownStatus,Limits,PullStartedAt,PullStoppedAt,ExecutionStoppedAt,AvailabilityZone,LaunchType,Containers{DockerId,Name,DockerName,KnownStatus,DesiredStatus,CreatedAt,StartedAt,FinishedAt,ExitCode,Image,ImageID,Labels,Type,LogDriver,LogOptions,Limits,ContainerARN,Networks}}}"
	containerStatsQueryGQL    = "{Container{Stats}}"
	containerNetworksQueryGQL = "{Container{Networks}}"
	tagsQueryGQL              = "{Task{ContainerInstanceTags,TaskTags}}"
)

func tagsQueryHelper(ecsClient *mock_api.MockECSClient) {
	contInstTag1Key := "ContainerInstanceTag1"
	contInstTag1Val := "firstTag"
	contInstTag2Key := "ContainerInstanceTag2"
	contInstTag2Val := "secondTag"
	taskTag1Key := "TaskTag1"
	taskTag1Val := "firstTag"
	taskTag2Key := "TaskTag2"
	taskTag2Val := "secondTag"

	ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return([]*ecs.Tag{
		&ecs.Tag{
			Key:   &contInstTag1Key,
			Value: &contInstTag1Val,
		},
		&ecs.Tag{
			Key:   &contInstTag2Key,
			Value: &contInstTag2Val,
		},
	}, nil).AnyTimes()
	ecsClient.EXPECT().GetResourceTags(taskARN).Return([]*ecs.Tag{
		&ecs.Tag{
			Key:   &taskTag1Key,
			Value: &taskTag1Val,
		},
		&ecs.Tag{
			Key:   &taskTag2Key,
			Value: &taskTag2Val,
		},
	}, nil)
}
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
				MacAddress:               macAddress,
				PrivateDNSName:           privateDNSName,
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

	pulledContainer := &apicontainer.Container{
		Name:                pulledContainerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerPulled,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		ContainerArn:        "arn:aws:ecs:ap-northnorth-1:NNN:container/NNNNNNNN-aaaa-4444-bbbb-00000000000",
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
	pulledDockerContainer := &apicontainer.DockerContainer{
		Container: pulledContainer,
	}
	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}
	pulledContainerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: pulledDockerContainer,
	}

	dockerStats.NumProcs = 2
	testCases := []struct {
		name                 string
		query                string
		taskContainerError   bool
		containerCTXError    bool
		taskCTXError         bool
		taskByIdError        bool
		dockerStatsError     bool
		tagsError            bool
		pulledContainerError bool
		expectedResponse     gqlResponse
	}{
		{
			name:                 "Conainer Query",
			query:                containerQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedContainerResponse,
		},
		{
			name:                 "Task Query",
			query:                taskQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedTaskResponse,
		},
		{
			name:                 "Conainer Stats Query",
			query:                containerStatsQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedContainerStatsResponse,
		},
		{
			name:                 "Tags Query",
			query:                tagsQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedTagsResponse,
		},
		{
			name:                 "No Container Task Query",
			query:                taskQueryGQL,
			taskContainerError:   true,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedContainerTaskResponse,
		},
		{
			name:                 "Get Container ctx Error",
			query:                containerQueryGQL,
			taskContainerError:   false,
			containerCTXError:    true,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedContainerCTXResponse,
		},
		{
			name:                 "Get Task ctx Error",
			query:                taskQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         true,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedTaskCTXResponse,
		},
		{
			name:                 "Get Task by ID Error (Stats Query)",
			query:                containerStatsQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        true,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedTaskByIDResponse,
		},
		{
			name:                 "Get Task by ID Error (Network Query)",
			query:                containerNetworksQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        true,
			dockerStatsError:     false,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedTaskByIDResponse,
		},
		{
			name:                 "Get Container Stats Error",
			query:                containerStatsQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     true,
			tagsError:            false,
			pulledContainerError: false,
			expectedResponse:     expectedFailedStatsResponse,
		},
		{
			name:                 "Get Tags Error",
			query:                tagsQueryGQL,
			taskContainerError:   false,
			containerCTXError:    false,
			taskCTXError:         false,
			taskByIdError:        false,
			dockerStatsError:     false,
			tagsError:            true,
			pulledContainerError: false,
			expectedResponse:     expectedFailedTagsResponse,
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
				ctx = context.WithValue(context.Background(), Container, dockerContainer)
				ctx = context.WithValue(ctx, Task, "task")
			} else {
				ctx = context.WithValue(context.Background(), Container, dockerContainer)
				ctx = context.WithValue(ctx, Task, task)
			}

			if tc.taskContainerError {
				state.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)
			} else if tc.dockerStatsError {
				state.EXPECT().TaskByID(containerID).Return(task, true)
				statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(nil, nil,
					errors.Errorf("stats engine: task '%s' for container '%s' not found", taskARN, containerID))
			} else if tc.taskByIdError {
				state.EXPECT().TaskByID(containerID).Return(nil, false)
			} else if tc.tagsError {
				ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return(nil, errors.New("Mock Error")).AnyTimes()
				ecsClient.EXPECT().GetResourceTags(taskARN).Return(nil, errors.New("Mock Error")).AnyTimes()
			} else if tc.name == "Tags Query" {
				tagsQueryHelper(ecsClient)
			} else {
				statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, &stats.NetworkStatsPerSec{}, nil).AnyTimes()
				state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true).AnyTimes()
				state.EXPECT().PulledContainerMapByArn(taskARN).Return(pulledContainerNameToDockerContainer, true).AnyTimes()
				state.EXPECT().TaskByID(containerID).Return(task, true).AnyTimes()
			}
			params := graphql.Params{
				Schema:        schema,
				RequestString: tc.query,
				Context:       ctx,
			}
			res := graphql.Do(params)
			if tc.taskContainerError || tc.containerCTXError || tc.taskCTXError || tc.taskByIdError ||
				tc.dockerStatsError || tc.tagsError || tc.pulledContainerError {
				assert.Equal(t, len(tc.expectedResponse.Errors), len(res.Errors))
				for _, error := range res.Errors {
					assert.Contains(t, tc.expectedResponse.Errors, error.Message)
				}
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
