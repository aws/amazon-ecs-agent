// +build windows,unit

// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package credentialspec

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"

	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN                = "task1"
	executionCredentialsID = "exec-creds-id"
)

func TestClearCredentialSpecDataHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)

	credSpecMapData := map[string]string{
		"credentialspec:file://localfilePath.json": "credentialspec=file://localfilePath.json",
		"credentialspec:s3ARN":                     "credentialspec=file://ResourceDir/s3_taskARN_fileName.json",
		"credentialspec:ssmARN":                    "credentialspec=file://ResourceDir/ssm_taskARN_fileName.json",
	}

	credspecRes := &CredentialSpecResource{
		CredSpecMap: credSpecMapData,
		os:          mockOS,
	}

	gomock.InOrder(
		mockOS.EXPECT().Remove(gomock.Any()).Return(nil).AnyTimes(),
	)

	err := credspecRes.Cleanup()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(credspecRes.CredSpecMap))
}

func TestClearCredentialSpecDataErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOS := mock_oswrapper.NewMockOS(ctrl)

	credSpecMapData := map[string]string{
		"credentialspec:file://localfilePath.json": "credentialspec=file://localfilePath.json",
		"credentialspec:s3ARN":                     "credentialspec=file://ResourceDir/s3_taskARN_fileName.json",
		"credentialspec:ssmARN":                    "credentialspec=file://ResourceDir/ssm_taskARN_fileName.json",
	}

	credspecRes := &CredentialSpecResource{
		CredSpecMap: credSpecMapData,
		os:          mockOS,
	}

	gomock.InOrder(
		mockOS.EXPECT().Remove(gomock.Any()).Return(nil).Times(2),
		mockOS.EXPECT().Remove(gomock.Any()).Return(errors.New("test error")),
	)

	err := credspecRes.Cleanup()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(credspecRes.CredSpecMap))
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	credspecRes := &CredentialSpecResource{
		knownStatusUnsafe:   resourcestatus.ResourceCreated,
		desiredStatusUnsafe: resourcestatus.ResourceCreated,
	}
	credspecRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			S3ClientCreator:    s3ClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)
	assert.Equal(t, resourcestatus.ResourceStatusNone, credspecRes.GetKnownStatus())
	assert.Equal(t, resourcestatus.ResourceCreated, credspecRes.GetDesiredStatus())

}

func TestMarshalUnmarshalJSON(t *testing.T) {
	requiredCredentialSpecs := make(map[string][]*apicontainer.Container)
	testCredSpec := "credentialspec:file://test.json"
	testContainer := &apicontainer.Container{
		Name: "test-container",
	}

	requiredCredentialSpecs[testCredSpec] = append(requiredCredentialSpecs[testCredSpec], testContainer)

	credspecIn := &CredentialSpecResource{
		taskARN:                 taskARN,
		executionCredentialsID:  executionCredentialsID,
		createdAt:               time.Now(),
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredentialSpecs,
	}

	bytes, err := json.Marshal(credspecIn)
	require.NoError(t, err)

	credSpecOut := &CredentialSpecResource{}
	err = json.Unmarshal(bytes, credSpecOut)
	require.NoError(t, err)
	assert.Equal(t, credspecIn.taskARN, credSpecOut.taskARN)
	assert.WithinDuration(t, credspecIn.createdAt, credSpecOut.createdAt, time.Microsecond)
	assert.Equal(t, credspecIn.desiredStatusUnsafe, credSpecOut.desiredStatusUnsafe)
	assert.Equal(t, credspecIn.knownStatusUnsafe, credSpecOut.knownStatusUnsafe)
	assert.Equal(t, credspecIn.executionCredentialsID, credSpecOut.executionCredentialsID)
	assert.Equal(t, len(credspecIn.requiredCredentialSpecs), len(credSpecOut.requiredCredentialSpecs))
}
