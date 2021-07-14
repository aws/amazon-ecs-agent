// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package gql

import (
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	v4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/pkg/errors"
)

var ContainerMetadataPath = "/graphql/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx)

func CreateSchema(
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	containerInstanceArn string) (graphql.Schema, error) {

	var containerType = graphql.NewObject(graphql.ObjectConfig{
		Name: "Container",
		Fields: graphql.Fields{
			"DockerId": &graphql.Field{
				Type: graphql.String,
			},
			"Name": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerNameResolver,
			},
			"DockerName": &graphql.Field{
				Type: graphql.String,
			},
			"Image": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerImageResolver,
			},
			"ImageID": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerImageIDResolver,
			},
			"Labels": &graphql.Field{
				Type:    JSON,
				Resolve: containerLabelsResolver,
			},
			"DesiredStatus": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerDesiredStatusResolver,
			},
			"KnownStatus": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerKnownStatusResolver,
			},
			"Limits": &graphql.Field{
				Type:    JSON,
				Resolve: containerLimitsResolver,
			},
			"ExitCode": &graphql.Field{
				Type:    graphql.Int,
				Resolve: exitCodeResolver,
			},
			"CreatedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerCreatedAtResolver,
			},
			"StartedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerStartedAtResolver,
			},
			"FinishedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerFinishedAtResolver,
			},
			"Type": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerTypeResolver,
			},
			"LogDriver": &graphql.Field{
				Type:    graphql.String,
				Resolve: logDriverResolver,
			},
			"LogOptions": &graphql.Field{
				Type:    JSON,
				Resolve: logOptionsResolver,
			},
			"ContainerARN": &graphql.Field{
				Type:    graphql.String,
				Resolve: containerARNResolver,
			},
			"Stats": &graphql.Field{
				Type:    JSON,
				Resolve: containerStatsResolver(state, statsEngine),
			},
			"Networks": &graphql.Field{
				Type:    graphql.NewList(JSON),
				Resolve: containerNetworksResolver(state),
			},
		},
	})

	var taskType = graphql.NewObject(graphql.ObjectConfig{
		Name: "Task",
		Fields: graphql.Fields{
			"Cluster": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return cluster, nil
				},
			},
			"TaskARN": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskARNResolver,
			},
			"Family": &graphql.Field{
				Type: graphql.String,
			},
			"Revision": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskRevisionResolver,
			},
			"DesiredStatus": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskDesiredStatusResolver,
			},
			"KnownStatus": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskKnownStatusResolver,
			},
			"Limits": &graphql.Field{
				Type:    JSON,
				Resolve: taskLimitsResolver,
			},
			"PullStartedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskPullStartedResolver,
			},
			"PullStoppedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskPullStoppedResolver,
			},
			"ExecutionStoppedAt": &graphql.Field{
				Type:    graphql.String,
				Resolve: taskExecutionStoppedResolver,
			},
			"AvailabilityZone": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return availabilityZone, nil
				},
			},
			"LaunchType": &graphql.Field{
				Type: graphql.String,
			},
			"Containers": &graphql.Field{
				Type:    graphql.NewList(containerType),
				Resolve: taskContainersResolver(state),
			},
			"TaskTags": &graphql.Field{
				Type:    JSON,
				Resolve: taskTagsResolver(ecsClient),
			},
			"ContainerInstanceTags": &graphql.Field{
				Type:    JSON,
				Resolve: containerInstanceTagsResolver(containerInstanceArn, ecsClient),
			},
		},
	})

	// Create GraphQL Schema
	fields := graphql.Fields{
		"Container": &graphql.Field{
			Type:    containerType,
			Resolve: containerResolver,
		},
		"Task": &graphql.Field{
			Type:    taskType,
			Resolve: taskResolver,
		},
	}
	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}

	return graphql.NewSchema(schemaConfig)
}

func containerResolver(p graphql.ResolveParams) (interface{}, error) {
	dockerContainer, ok := p.Context.Value(Container).(*apicontainer.DockerContainer)
	if !ok {
		return nil, errors.Errorf("Could not cast to container")
	}

	return dockerContainer, nil
}

func containerNameResolver(p graphql.ResolveParams) (interface{}, error) {
	dockerContainer, ok := p.Source.(*apicontainer.DockerContainer)
	if !ok {
		return "", nil
	}
	return dockerContainer.Container.Name, nil
}

func containerImageResolver(p graphql.ResolveParams) (interface{}, error) {
	dockerContainer, ok := p.Source.(*apicontainer.DockerContainer)
	if !ok {
		return "", nil
	}
	return dockerContainer.Container.Image, nil
}

func containerImageIDResolver(p graphql.ResolveParams) (interface{}, error) {
	dockerContainer, ok := p.Source.(*apicontainer.DockerContainer)
	if !ok {
		return "", nil
	}
	return dockerContainer.Container.ImageID, nil
}

func containerLabelsResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetLabels(), nil
	}
	return nil, nil
}

func containerDesiredStatusResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetDesiredStatus().String(), nil
	}
	return nil, nil
}

func containerKnownStatusResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetKnownStatus().String(), nil
	}
	return nil, nil
}

func containerLimitsResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		container := dockerContainer.Container
		containerCPU := v2.GetContainerCPULimit(container)
		return v2.LimitsResponse{
			CPU:    containerCPU,
			Memory: aws.Int64(int64(container.Memory)),
		}, nil
	}
	return nil, nil
}

// Container Field Resolvers
func exitCodeResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetKnownExitCode(), nil
	}
	return nil, nil
}

func timeResolverHelper(t time.Time) (interface{}, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func containerCreatedAtResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return timeResolverHelper(dockerContainer.Container.GetCreatedAt())
	}
	return nil, nil
}

func containerStartedAtResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return timeResolverHelper(dockerContainer.Container.GetStartedAt())
	}
	return nil, nil
}

func containerFinishedAtResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return timeResolverHelper(dockerContainer.Container.GetFinishedAt())
	}
	return nil, nil
}

func containerTypeResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.Type.String(), nil
	}
	return nil, nil
}

func logDriverResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetLogDriver(), nil
	}
	return nil, nil
}

func logOptionsResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.GetLogOptions(), nil
	}
	return nil, nil
}

func containerARNResolver(p graphql.ResolveParams) (interface{}, error) {
	if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
		return dockerContainer.Container.ContainerArn, nil
	}
	return nil, nil
}

func containerStatsResolver(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		dockerContainer, ok := p.Source.(*apicontainer.DockerContainer)
		if !ok {
			return nil, nil
		}
		task, ok := state.TaskByID(dockerContainer.DockerID)
		if !ok {
			return nil, errors.Errorf("Unable to find task for container '%s'", dockerContainer.DockerID)
		}

		dockerStats, network_rate_stats, err := statsEngine.ContainerDockerStats(task.Arn, dockerContainer.DockerID)
		if err != nil {
			return nil, err
		}

		return v4.StatsResponse{
			StatsJSON:          dockerStats,
			Network_rate_stats: network_rate_stats,
		}, nil
	}
}

func containerNetworksResolver(state dockerstate.TaskEngineState) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if dockerContainer, ok := p.Source.(*apicontainer.DockerContainer); ok {
			// Pulled containers do not have a network
			// TODO: Find a cleaner way to have pulled containers return nil
			if dockerContainer.Container.KnownStatusUnsafe == apicontainerstatus.ContainerPulled {
				return nil, nil
			}
			task, ok := state.TaskByID(dockerContainer.DockerID)
			if !ok {
				return nil, errors.Errorf("Unable to find task for container '%s'", dockerContainer.DockerID)
			}
			eni := task.GetPrimaryENI()
			var resp []v4.Network
			if eni != nil { // NetworkMode is AWSVPC
				props, err := v4.NewNetworkInterfaceProperties(task)
				if err != nil {
					return nil, err
				}
				resp = []v4.Network{
					{
						Network: containermetadata.Network{
							NetworkMode:   utils.NetworkModeAWSVPC,
							IPv4Addresses: eni.GetIPV4Addresses(),
							IPv6Addresses: eni.GetIPV6Addresses(),
						},
						NetworkInterfaceProperties: props,
					},
				}
				return resp, nil
			}
			resp, err := v4.GetContainerNetworkMetadata(dockerContainer.DockerID, state)
			if err != nil {
				return nil, err
			}
			return resp, nil
		}
		return nil, nil
	}
}

// Task Field Resolvers
func taskResolver(p graphql.ResolveParams) (interface{}, error) {
	task, ok := p.Context.Value(Task).(*apitask.Task)
	if !ok {
		return nil, errors.New("Could not cast to task")
	}

	return task, nil
}
func taskARNResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return task.Arn, nil
	}
	return nil, nil
}

func taskRevisionResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return task.Version, nil
	}
	return nil, nil
}

func taskDesiredStatusResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return task.GetDesiredStatus().String(), nil
	}
	return nil, nil
}

func taskKnownStatusResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return task.GetKnownStatus().String(), nil
	}
	return nil, nil
}

func taskLimitsResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		taskCPU := task.CPU
		taskMemory := task.Memory
		if taskCPU != 0 || taskMemory != 0 {
			taskLimits := v2.GetTaskLimits(taskCPU, taskMemory)
			return taskLimits, nil
		}
		return nil, nil
	}
	return nil, nil
}

func taskPullStartedResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return aws.Time(task.GetPullStartedAt().UTC()).Format(time.RFC3339Nano), nil
	}
	return nil, nil
}

func taskPullStoppedResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return aws.Time(task.GetPullStoppedAt().UTC()).Format(time.RFC3339Nano), nil
	}
	return nil, nil
}

func taskExecutionStoppedResolver(p graphql.ResolveParams) (interface{}, error) {
	if task, ok := p.Source.(*apitask.Task); ok {
		return aws.Time(task.GetExecutionStoppedAt().UTC()).Format(time.RFC3339Nano), nil
	}
	return nil, nil
}

func taskContainersResolver(state dockerstate.TaskEngineState) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if task, ok := p.Source.(*apitask.Task); ok {
			containerNameToDockerContainer, ok := state.ContainerMapByArn(task.Arn)
			if !ok {
				return "", errors.Errorf("Unable to get container name mapping for task %v",
					task.Arn)
			}
			resp := []*apicontainer.DockerContainer{}
			for _, dockerContainer := range containerNameToDockerContainer {
				resp = append(resp, dockerContainer)
			}
			pulledContainers, _ := state.PulledContainerMapByArn(task.Arn)
			for _, dockerContainer := range pulledContainers {
				resp = append(resp, dockerContainer)
			}
			return resp, nil
		}
		return nil, nil
	}
}

func taskTagsResolver(ecsClient api.ECSClient) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if task, ok := p.Source.(*apitask.Task); ok {
			taskTags, err := ecsClient.GetResourceTags(task.Arn)
			if err != nil {
				return nil, errors.Errorf("Task Metadata error: unable to get TaskTags for '%s': %s", task.Arn, err.Error())
			}
			tagMap := make(map[string]string)
			for _, tag := range taskTags {
				tagMap[*tag.Key] = *tag.Value
			}
			return tagMap, nil
		}
		return nil, nil
	}
}

func containerInstanceTagsResolver(containerInstancArn string, ecsClient api.ECSClient) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		containerTags, err := ecsClient.GetResourceTags(containerInstancArn)
		if err != nil {
			return nil, errors.Errorf("Task Metadata error: unable to get ContainerInstanceTags for '%s': %s", containerInstancArn, err.Error())
		}
		tagMap := make(map[string]string)
		for _, tag := range containerTags {
			tagMap[*tag.Key] = *tag.Value
		}
		return tagMap, nil
	}
}

// Adapted from https://github.com/graphql-go/graphql/issues/298
// Creates Custom JSON Scalar Type
func parseLiteral(astValue ast.Value) interface{} {
	kind := astValue.GetKind()

	switch kind {
	case kinds.StringValue:
		return astValue.GetValue()
	case kinds.BooleanValue:
		return astValue.GetValue()
	case kinds.IntValue:
		return astValue.GetValue()
	case kinds.FloatValue:
		return astValue.GetValue()
	case kinds.ObjectValue:
		obj := make(map[string]interface{})
		for _, v := range astValue.GetValue().([]*ast.ObjectField) {
			obj[v.Name.Value] = parseLiteral(v.Value)
		}
		return obj
	case kinds.ListValue:
		list := make([]interface{}, 0)
		for _, v := range astValue.GetValue().([]ast.Value) {
			list = append(list, parseLiteral(v))
		}
		return list
	default:
		return nil
	}
}

// Addapted From https://github.com/graphql-go/graphql/issues/298
var JSON = graphql.NewScalar(
	graphql.ScalarConfig{
		Name:        "JSON",
		Description: "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf)",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: parseLiteral,
	},
)
