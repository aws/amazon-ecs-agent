// +build windows,unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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
package engine

import (
	"context"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	task := &apitask.Task{}

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &defaultConfig,
		ctx:   ctx,
	}

	gomock.InOrder(
		mockState.EXPECT().RemoveTask(task),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}

func TestCredentialSpecResourceTaskFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	// metadata required for createContainer workflow validation
	credentialSpecTaskARN := "credentialSpecTask"
	credentialSpecTaskFamily := "credentialSpecFamily"
	credentialSpecTaskVersion := "1"
	credentialSpecTaskContainerName := "credentialSpecContainer"

	c := &apicontainer.Container{
		Name: credentialSpecTaskContainerName,
	}
	credentialspecFile := "credentialspec:file://gmsa_gmsa-acct.json"
	targetCredentialspecFile := "credentialspec=file://gmsa_gmsa-acct.json"
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c.DockerConfig.HostConfig = &hostConfig

	// sample test
	testTask := &apitask.Task{
		Arn:        credentialSpecTaskARN,
		Family:     credentialSpecTaskFamily,
		Version:    credentialSpecTaskVersion,
		Containers: []*apicontainer.Container{c},
	}

	// metadata required for execution role authentication workflow
	credentialsID := "execution role"

	// configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)

	// validate base config
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}

	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                credentialSpecTaskARN,
		"com.amazonaws.ecs.container-name":          credentialSpecTaskContainerName,
		"com.amazonaws.ecs.task-definition-family":  credentialSpecTaskFamily,
		"com.amazonaws.ecs.task-definition-version": credentialSpecTaskVersion,
		"com.amazonaws.ecs.cluster":                 "",
	}

	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)

	credentialSpecReq := []string{credentialspecFile}

	credentialSpecRes, cerr := credentialspec.NewCredentialSpecResource(
		testTask.Arn,
		defaultConfig.AWSRegion,
		credentialSpecReq,
		credentialsID,
		credentialsManager,
		ssmClientCreator,
		s3ClientCreator)
	assert.NoError(t, cerr)

	credSpecdata := map[string]string{
		credentialspecFile: targetCredentialspecFile,
	}
	credentialSpecRes.CredSpecMap = credSpecdata

	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
		credentialspec.ResourceName: {credentialSpecRes},
	}

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()

	client.EXPECT().CreateContainer(gomock.Any(), expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())

	ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
	assert.Nil(t, ret.Error)
}

func TestCredentialSpecResourceTaskFileErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	// metadata required for createContainer workflow validation
	credentialSpecTaskARN := "credentialSpecTask"
	credentialSpecTaskFamily := "credentialSpecFamily"
	credentialSpecTaskVersion := "1"
	credentialSpecTaskContainerName := "credentialSpecContainer"

	c := &apicontainer.Container{
		Name: credentialSpecTaskContainerName,
	}
	credentialspecFile := "credentialspec:file://gmsa_gmsa-acct.json"
	targetCredentialspecFile := "credentialspec=file://gmsa_gmsa-acct.json"
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c.DockerConfig.HostConfig = &hostConfig

	// sample test
	testTask := &apitask.Task{
		Arn:        credentialSpecTaskARN,
		Family:     credentialSpecTaskFamily,
		Version:    credentialSpecTaskVersion,
		Containers: []*apicontainer.Container{c},
	}

	// metadata required for execution role authentication workflow
	credentialsID := "execution role"

	// configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)

	// validate base config
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}

	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                credentialSpecTaskARN,
		"com.amazonaws.ecs.container-name":          credentialSpecTaskContainerName,
		"com.amazonaws.ecs.task-definition-family":  credentialSpecTaskFamily,
		"com.amazonaws.ecs.task-definition-version": credentialSpecTaskVersion,
		"com.amazonaws.ecs.cluster":                 "",
	}

	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)

	credentialSpecReq := []string{credentialspecFile}

	credentialSpecRes, cerr := credentialspec.NewCredentialSpecResource(
		testTask.Arn,
		defaultConfig.AWSRegion,
		credentialSpecReq,
		credentialsID,
		credentialsManager,
		ssmClientCreator,
		s3ClientCreator)
	assert.NoError(t, cerr)

	credSpecdata := map[string]string{
		credentialspecFile: targetCredentialspecFile,
	}
	credentialSpecRes.CredSpecMap = credSpecdata

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()

	ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
	assert.Error(t, ret.Error)
}
