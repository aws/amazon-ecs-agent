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

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mock_ssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN                = "task1"
	executionCredentialsID = "exec-creds-id"
	testTempFile           = "testtempfile"
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
	assert.Equal(t, 1, len(credspecRes.CredSpecMap))
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
		mockOS.EXPECT().Remove(gomock.Any()).Return(nil).Times(1),
		mockOS.EXPECT().Remove(gomock.Any()).Return(errors.New("test error")),
	)

	err := credspecRes.Cleanup()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(credspecRes.CredSpecMap))
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
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	assert.NotNil(t, credspecRes.credentialsManager)
	assert.NotNil(t, credspecRes.ssmClientCreator)
	assert.NotNil(t, credspecRes.s3ClientCreator)
	assert.NotNil(t, credspecRes.resourceStatusToTransitionFunction)
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	testCredSpec := "credentialspec:file://test.json"
	targetCredSpec := "credentialspec=file://test.json"

	requiredCredentialSpecs := []string{testCredSpec}

	credSpecMap := map[string]string{}
	credSpecMap[testCredSpec] = targetCredSpec

	credspecIn := &CredentialSpecResource{
		taskARN:                 taskARN,
		executionCredentialsID:  executionCredentialsID,
		createdAt:               time.Now(),
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredentialSpecs,
		CredSpecMap:             credSpecMap,
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
	assert.Equal(t, len(credspecIn.CredSpecMap), len(credSpecOut.CredSpecMap))
	assert.EqualValues(t, credspecIn.CredSpecMap, credSpecOut.CredSpecMap)
}

func TestHandleCredentialspecFile(t *testing.T) {
	fileCredentialSpec := "credentialspec:file://test.json"
	expectedFileCredentialSpec := "credentialspec=file://test.json"

	requiredCredSpec := []string{fileCredentialSpec}

	cs := &CredentialSpecResource{
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
	}

	err := cs.handleCredentialspecFile(fileCredentialSpec)
	assert.NoError(t, err)

	targetCredentialSpecFile, err := cs.GetTargetMapping(fileCredentialSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
}

func TestHandleCredentialspecFileErr(t *testing.T) {
	fileCredentialSpec := "credentialspec:invalid-file://test.json"
	requiredCredSpec := []string{fileCredentialSpec}

	cs := &CredentialSpecResource{
		requiredCredentialSpecs: requiredCredSpec,
	}

	err := cs.handleCredentialspecFile(fileCredentialSpec)
	assert.Error(t, err)
}

func TestHandleSSMCredentialspecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	expectedFileCredentialSpec := "credentialspec=file://ssm_task1_test.json"

	requiredCredSpec := []string{ssmCredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "test-cred-spec-data"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			&ssm.Parameter{
				Name:  aws.String("test"),
				Value: aws.String(testData),
			},
		},
	}

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
		mockIO.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
	assert.NoError(t, err)

	targetCredentialSpecFile, err := cs.GetTargetMapping(ssmCredentialSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
}

func TestHandleSSMCredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{}
	credentialSpecSSMARN := "arn:aws:ssm:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	var termReason string
	cs := &CredentialSpecResource{
		terminalReason: termReason,
	}

	err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleSSMCredentialspecFileGetSSMParamErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	requiredCredSpec := []string{ssmCredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(nil, errors.New("test-error")).Times(1),
	)

	err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleSSMCredentialspecFileIOErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	requiredCredSpec := []string{ssmCredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "test-cred-spec-data"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			&ssm.Parameter{
				Name:  aws.String("test"),
				Value: aws.String(testData),
			},
		},
	}

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
		mockIO.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("test-error")),
	)

	err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleS3CredentialspecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"
	expectedFileCredentialSpec := "credentialspec=file://s3_task1_test.json"

	requiredCredSpec := []string{s3CredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
		os:                      mockOS,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3ClientForBucket(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(gomock.Any()).Return(nil),
		mockFile.EXPECT().Sync().Return(nil),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(gomock.Any(), gomock.Any()).Return(nil),
		mockFile.EXPECT().Close(),
	)

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.NoError(t, err)

	targetCredentialSpecFile, err := cs.GetTargetMapping(s3CredentialSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
}

func TestHandleS3CredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecS3ARN := "arn:aws:/test"
	s3CredentialSpec := "credentialspec:arn:/test"

	var termReason string
	cs := &CredentialSpecResource{
		terminalReason: termReason,
	}

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleS3CredentialspecFileS3ClientErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{}

	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"

	var termReason string
	cs := &CredentialSpecResource{
		terminalReason: termReason,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3ClientForBucket(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, errors.New("test-error")),
	)

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleS3CredentialspecFileWriteErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)

	iamCredentials := credentials.IAMRoleCredentials{}
	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"

	requiredCredSpec := []string{s3CredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
		os:                      mockOS,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3ClientForBucket(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(gomock.Any()).Return(nil),
		mockFile.EXPECT().Sync().Return(nil),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(gomock.Any(), gomock.Any()).Return(errors.New("test-error")),
		mockFile.EXPECT().Close(),
	)

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.Error(t, err)
}

func TestCreateSSM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)

	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	requiredCredSpec := []string{ssmCredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "test-cred-spec-data"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			&ssm.Parameter{
				Name:  aws.String("test"),
				Value: aws.String(testData),
			},
		},
	}

	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     "id",
			SecretAccessKey: "key",
		},
	}

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, true),
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
		mockIO.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	assert.NoError(t, cs.Create())
}

func TestCreateS3(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)

	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"

	requiredCredSpec := []string{s3CredentialSpec}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:       resourcestatus.ResourceCreated,
		desiredStatusUnsafe:     resourcestatus.ResourceCreated,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
		taskARN:                 taskARN,
		ioutil:                  mockIO,
		os:                      mockOS,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     "id",
			SecretAccessKey: "key",
		},
	}

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, true),
		s3ClientCreator.EXPECT().NewS3ClientForBucket(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(gomock.Any()).Return(nil),
		mockFile.EXPECT().Sync().Return(nil),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(gomock.Any(), gomock.Any()).Return(nil),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, cs.Create())
}

func TestCreateFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)

	fileCredentialSpec := "credentialspec:file://test.json"
	requiredCredSpec := []string{fileCredentialSpec}

	cs := &CredentialSpecResource{
		credentialsManager:      credentialsManager,
		requiredCredentialSpecs: requiredCredSpec,
		CredSpecMap:             map[string]string{},
	}

	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     "id",
			SecretAccessKey: "key",
		},
	}

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, true),
	)

	assert.NoError(t, cs.Create())
}

func TestGetName(t *testing.T) {
	cs := &CredentialSpecResource{}

	assert.Equal(t, ResourceName, cs.GetName())
}

func TestGetTargetMapping(t *testing.T) {
	inputCredSpec, outputCredSpec := "credentialspec:file://test.json", "credentialspec=file://test.json"
	credSpecMapData := map[string]string{
		inputCredSpec: outputCredSpec,
	}

	cs := &CredentialSpecResource{
		CredSpecMap: credSpecMapData,
	}

	targetCredSpec, err := cs.GetTargetMapping(inputCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, outputCredSpec, targetCredSpec)
}

func TestGetTargetMappingErr(t *testing.T) {
	cs := &CredentialSpecResource{
		CredSpecMap: map[string]string{},
	}

	targetCredSpec, err := cs.GetTargetMapping("testcredspec")
	assert.Error(t, err)
	assert.Empty(t, targetCredSpec)
}
