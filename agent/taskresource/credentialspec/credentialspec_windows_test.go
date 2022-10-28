//go:build windows && unit
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

package credentialspec

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks/s3manager"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mock_ssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN                = "arn:aws:ecs:us-west-2:123456789012:task/12345-678901234-56789"
	executionCredentialsID = "exec-creds-id"
	testTempFile           = "testtempfile"
)

func mockRename() func() {
	rename = func(oldpath, newpath string) error {
		return nil
	}

	return func() {
		rename = os.Rename
	}
}

func TestClearCredentialSpecDataHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credSpecMapData := map[string]string{
		"credentialspec:file://localfilePath.json": "credentialspec=file://localfilePath.json",
		"credentialspec:s3ARN":                     "credentialspec=file://s3_taskARN_fileName.json",
		"credentialspec:ssmARN":                    "credentialspec=file://ssm_taskARN_fileName.json",
	}

	credentialSpecResourceLocation := "C:/ProgramData/docker/credentialspecs/"
	credspecRes := &CredentialSpecResource{
		CredSpecMap:                    credSpecMapData,
		credentialSpecResourceLocation: credentialSpecResourceLocation,
	}

	err := credspecRes.Cleanup()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(credspecRes.CredSpecMap))
}

func TestClearCredentialSpecDataErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credSpecMapData := map[string]string{
		"credentialspec:file://localfilePath.json": "credentialspec=file://localfilePath.json",
		"credentialspec:ssmARN":                    "credentialspec=file://ssm_taskARN_fileName.json",
	}

	credentialSpecResourceLocation := "C:/ProgramData/docker/credentialspecs/"
	credspecRes := &CredentialSpecResource{
		CredSpecMap:                    credSpecMapData,
		credentialSpecResourceLocation: credentialSpecResourceLocation,
	}

	remove = func(name string) error {
		return errors.New("test-error")
	}
	defer func() {
		remove = os.Remove
	}()

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

	credentialSpecContainerMap := map[string]string{testCredSpec: "windowsServerCore"}

	credSpecMap := map[string]string{}
	credSpecMap[testCredSpec] = targetCredSpec

	credspecIn := &CredentialSpecResource{
		taskARN:                    taskARN,
		executionCredentialsID:     executionCredentialsID,
		createdAt:                  time.Now(),
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		credentialSpecContainerMap: credentialSpecContainerMap,
		CredSpecMap:                credSpecMap,
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
	assert.Equal(t, len(credspecIn.credentialSpecContainerMap), len(credSpecOut.credentialSpecContainerMap))
	assert.Equal(t, len(credspecIn.CredSpecMap), len(credSpecOut.CredSpecMap))
	assert.EqualValues(t, credspecIn.CredSpecMap, credSpecOut.CredSpecMap)
}

func TestHandleCredentialspecFile(t *testing.T) {
	fileCredentialSpec := "credentialspec:file://test.json"
	expectedFileCredentialSpec := "credentialspec=file://test.json"

	credentialSpecContainerMap := map[string]string{fileCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		credentialSpecContainerMap: credentialSpecContainerMap,
		CredSpecMap:                map[string]string{},
	}

	err := cs.handleCredentialspecFile(fileCredentialSpec)
	assert.NoError(t, err)

	targetCredentialSpecFile, err := cs.GetTargetMapping(fileCredentialSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
}

func TestHandleCredentialspecFileErr(t *testing.T) {
	fileCredentialSpec := "credentialspec:invalid-file://test.json"
	credentialSpecContainerMap := map[string]string{fileCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		credentialSpecContainerMap: credentialSpecContainerMap,
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
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	containerName := "webapp"

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	customCredSpecFileName := fmt.Sprintf("%s%s%s", "12345-678901234-56789", containerName, credentialSpecSSMARN)
	hasher := sha256.New()
	hasher.Write([]byte(customCredSpecFileName))
	customCredSpecFileName = fmt.Sprintf("%x", hasher.Sum(nil))
	expectedFileCredentialSpec := fmt.Sprintf("credentialspec=file://%s", customCredSpecFileName)

	credentialSpecContainerMap := map[string]string{
		ssmCredentialSpec: containerName,
	}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
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

func TestHandleSSMCredentialspecFileWithHierarchicalPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/x/y/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	customCredSpecFileName := fmt.Sprintf("%s%s%s", "12345-678901234-56789", containerName, credentialSpecSSMARN)
	hasher := sha256.New()
	hasher.Write([]byte(customCredSpecFileName))
	customCredSpecFileName = fmt.Sprintf("%x", hasher.Sum(nil))
	expectedFileCredentialSpec := fmt.Sprintf("credentialspec=file://%s", customCredSpecFileName)

	credentialSpecContainerMap := map[string]string{
		ssmCredentialSpec: containerName,
	}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
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
				Name:  aws.String("x/y/test"),
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
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
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
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{ssmCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		credentialSpecContainerMap: credentialSpecContainerMap,
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
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{ssmCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
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

func TestHandlerSSMCredentialspecCredMissingErr(t *testing.T) {
	cs := &CredentialSpecResource{}

	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	iamCredentials := credentials.IAMRoleCredentials{}

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
	mockFile := mock_oswrapper.NewMockFile()
	mockS3Client := mock_s3.NewMockS3ManagerClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"
	expectedFileCredentialSpec := "credentialspec=file://s3_12345-678901234-56789_test"

	credentialSpecContainerMap := map[string]string{s3CredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	defer mockRename()()
	mockFile.(*mock_oswrapper.MockFile).NameImpl = func() string {
		return testTempFile
	}
	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
	)

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.NoError(t, err)

	targetCredentialSpecFile, err := cs.GetTargetMapping(s3CredentialSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
}

func TestHandleS3CredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
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
	mockS3Client := mock_s3.NewMockS3ManagerClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
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
		s3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, errors.New("test-error")),
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
	mockFile := mock_oswrapper.NewMockFile()
	mockS3Client := mock_s3.NewMockS3ManagerClient(ctrl)

	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"

	credentialSpecContainerMap := map[string]string{s3CredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	mockFile.(*mock_oswrapper.MockFile).NameImpl = func() string {
		return testTempFile
	}

	rename = func(oldpath, newpath string) error {
		return errors.New("test-error")
	}
	defer func() {
		rename = os.Rename
	}()

	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
	)

	err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandlerS3CredentialspecCredMissingErr(t *testing.T) {
	cs := &CredentialSpecResource{}

	credentialSpecS3ARN := "arn:aws:s3:::bucket_name/test"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"
	iamCredentials := credentials.IAMRoleCredentials{}

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
	credentialSpecContainerMap := map[string]string{ssmCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
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
	mockFile := mock_oswrapper.NewMockFile()
	mockS3Client := mock_s3.NewMockS3ManagerClient(ctrl)

	s3CredentialSpec := "credentialspec:arn:aws:s3:::bucket_name/test"

	credentialSpecContainerMap := map[string]string{s3CredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		ioutil:                     mockIO,
		credentialSpecContainerMap: credentialSpecContainerMap,
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

	defer mockRename()()
	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, true),
		s3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockIO.EXPECT().TempFile(gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
	)

	assert.NoError(t, cs.Create())
}

func TestCreateFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)

	fileCredentialSpec := "credentialspec:file://test.json"
	credentialSpecContainerMap := map[string]string{fileCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		credentialsManager:         credentialsManager,
		CredSpecMap:                map[string]string{},
		credentialSpecContainerMap: credentialSpecContainerMap,
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
