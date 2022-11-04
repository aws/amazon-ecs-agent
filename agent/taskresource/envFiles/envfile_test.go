//go:build unit
// +build unit

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

package envFiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks/s3manager"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_bufio "github.com/aws/amazon-ecs-agent/agent/utils/bufiowrapper/mocks"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	executionCredentialsID = "exec-creds-id"
	region                 = "us-west-2"
	cluster                = "testCluster"
	taskARN                = "arn:aws:ecs:us-east-2:01234567891011:task/testCluster/abcdef12-34gh-idkl-mno5-pqrst6789"
	resourceDir            = "resourceDir"
	iamRoleARN             = "iamRoleARN"
	accessKeyId            = "accessKey"
	secretAccessKey        = "secret"
	s3Bucket               = "s3Bucket"
	s3Path                 = "path" + string(filepath.Separator) + "to" + string(filepath.Separator) + "envfile"
	s3File                 = "s3key.env"
	s3Key                  = s3Path + string(filepath.Separator) + s3File
	tempFile               = "tmp_file"
)

func setup(t *testing.T) (oswrapper.File, *mock_ioutilwrapper.MockIOUtil,
	*mock_credentials.MockManager, *mock_factory.MockS3ClientCreator, *mock_s3.MockS3ManagerClient, func()) {
	ctrl := gomock.NewController(t)

	mockFile := mock_oswrapper.NewMockFile()
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockCredentialsManager := mock_credentials.NewMockManager(ctrl)
	mockS3ClientCreator := mock_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3ManagerClient(ctrl)

	return mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, ctrl.Finish
}

func newMockEnvfileResource(envfileLocations []container.EnvironmentFile, mockCredentialsManager *mock_credentials.MockManager,
	mockS3ClientCreator *mock_factory.MockS3ClientCreator,
	mockIOUtil *mock_ioutilwrapper.MockIOUtil) *EnvironmentFileResource {
	return &EnvironmentFileResource{
		cluster:                cluster,
		taskARN:                taskARN,
		region:                 region,
		resourceDir:            resourceDir,
		environmentFilesSource: envfileLocations,
		executionCredentialsID: executionCredentialsID,
		credentialsManager:     mockCredentialsManager,
		s3ClientCreator:        mockS3ClientCreator,
		ioutil:                 mockIOUtil,
	}
}

func sampleEnvironmentFile(value, envfileType string) container.EnvironmentFile {
	return container.EnvironmentFile{
		Value: value,
		Type:  envfileType,
	}
}

func TestInitializeFileEnvResource(t *testing.T) {
	_, _, mockCredentialsManager, _, _, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, nil, nil)
	envfileResource.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: mockCredentialsManager,
		},
	}, status.TaskRunning, status.TaskRunning)

	assert.NotNil(t, envfileResource.statusToTransitions)
	assert.Equal(t, 1, len(envfileResource.statusToTransitions))
	assert.NotNil(t, envfileResource.credentialsManager)
	assert.NotNil(t, envfileResource.s3ClientCreator)
	assert.NotNil(t, envfileResource.ioutil)
}

func TestCreateWithEnvVarFile(t *testing.T) {
	mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	rename = func(oldpath, newpath string) error {
		return nil
	}
	defer func() {
		rename = os.Rename
	}()

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Do(
			func(ctx aws.Context, w io.WriterAt, input *s3.GetObjectInput) {
				assert.Equal(t, s3Bucket, aws.StringValue(input.Bucket))
				assert.Equal(t, s3Key, aws.StringValue(input.Key))
			}).Return(int64(0), nil),
	)

	assert.NoError(t, envfileResource.Create())
}

func TestCreateWithInvalidS3ARN(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s", s3File), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "unable to parse bucket and key from s3 ARN specified in environmentFile")
}

func TestCreateUnableToRetrieveDataFromS3(t *testing.T) {
	mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), errors.New("error response")),
	)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "error response")
}

func TestCreateUnableToCreateTmpFile(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(nil, errors.New("error response")),
	)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "error response")
}

func TestCreateRenameFileError(t *testing.T) {
	mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	rename = func(oldpath, newpath string) error {
		return errors.New("error response")
	}
	defer func() {
		rename = os.Rename
	}()

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
	)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "error response")
}

func TestEnvFileCleanupSuccess(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)

	assert.NoError(t, envfileResource.Cleanup())
}

func TestEnvFileCleanupResourceDirRemoveFail(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil)

	removeAll = func(path string) error {
		return errors.New("error response")
	}
	defer func() {
		removeAll = os.RemoveAll
	}()

	assert.Error(t, envfileResource.Cleanup())
}

func TestReadEnvVarsFromEnvfiles(t *testing.T) {
	mockFile, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	ctrl := gomock.NewController(t)
	mockBufio := mock_bufio.NewMockBufio(ctrl)
	mockScanner := mock_bufio.NewMockScanner(ctrl)

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil)
	envfileResource.bufio = mockBufio

	envfileContentLine1 := "key1=value"
	envFileContentLine2 := "key2=val1=val2"

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		open = tempOpen
	}()
	gomock.InOrder(
		mockBufio.EXPECT().NewScanner(mockFile).Return(mockScanner),
		mockScanner.EXPECT().Scan().Return(true),
		mockScanner.EXPECT().Text().Return(envfileContentLine1),
		mockScanner.EXPECT().Scan().Return(true),
		mockScanner.EXPECT().Text().Return(envFileContentLine2),
		mockScanner.EXPECT().Scan().Return(false),
		mockScanner.EXPECT().Err().Return(nil),
	)

	envVarsList, err := envfileResource.ReadEnvVarsFromEnvfiles()

	assert.Nil(t, err)
	assert.Equal(t, 1, len(envVarsList))
	assert.Equal(t, "value", envVarsList[0]["key1"])
	assert.Equal(t, "val1=val2", envVarsList[0]["key2"])
}

func TestReadEnvVarsCommentFromEnvfiles(t *testing.T) {
	mockFile, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	ctrl := gomock.NewController(t)
	mockBufio := mock_bufio.NewMockBufio(ctrl)
	mockScanner := mock_bufio.NewMockScanner(ctrl)

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil)
	envfileResource.bufio = mockBufio

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		open = tempOpen
	}()

	envfileContentComment := "# some comment here"
	gomock.InOrder(
		mockBufio.EXPECT().NewScanner(mockFile).Return(mockScanner),
		mockScanner.EXPECT().Scan().Return(true),
		mockScanner.EXPECT().Text().Return(envfileContentComment),
		mockScanner.EXPECT().Scan().Return(false),
		mockScanner.EXPECT().Err().Return(nil),
	)

	envVarsList, err := envfileResource.ReadEnvVarsFromEnvfiles()

	assert.Nil(t, err)
	assert.Equal(t, 0, len(envVarsList[0]))
}

func TestReadEnvVarsInvalidFromEnvfiles(t *testing.T) {
	mockFile, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	ctrl := gomock.NewController(t)
	mockBufio := mock_bufio.NewMockBufio(ctrl)
	mockScanner := mock_bufio.NewMockScanner(ctrl)

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil)
	envfileResource.bufio = mockBufio

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		open = tempOpen
	}()

	envfileContentInvalid := "=value"
	gomock.InOrder(
		mockBufio.EXPECT().NewScanner(mockFile).Return(mockScanner),
		mockScanner.EXPECT().Scan().Return(true),
		mockScanner.EXPECT().Text().Return(envfileContentInvalid),
		mockScanner.EXPECT().Scan().Return(false),
		mockScanner.EXPECT().Err().Return(nil),
	)

	envVarsList, err := envfileResource.ReadEnvVarsFromEnvfiles()

	assert.Nil(t, err)
	assert.Equal(t, 0, len(envVarsList[0]))
}

func TestReadEnvVarsUnableToReadEnvfile(t *testing.T) {
	_, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf("arn:aws:s3:::%s/%s", s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil)

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return nil, errors.New("error response")
	}
	defer func() {
		open = tempOpen
	}()

	_, err := envfileResource.ReadEnvVarsFromEnvfiles()

	assert.NotNil(t, err)
}
