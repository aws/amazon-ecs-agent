// +build linux,unit
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

package firelens

import (
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
)

const (
	testCluster                = "mycluster"
	testTaskARN                = "arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a"
	testTaskDefinition         = "taskdefinition:1"
	testEC2InstanceID          = "i-123456789a"
	testDataDir                = "testdatadir"
	testResourceDir            = "testresourcedir"
	testTerminalResason        = "testterminalreason"
	testTempFile               = "testtempfile"
	testRegion                 = "us-west-2"
	testExecutionCredentialsID = "testexecutioncredentialsid"
	testExternalConfigType     = "testexternalconfigtype"
	testExternalConfigValue    = "testexternalconfigvalue"
)

var (
	testFirelensOptionsFile = map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "file",
		"config-file-value":       "/tmp/dummy.conf",
	}

	testFirelensOptionsS3 = map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "s3",
		"config-file-value":       "arn:aws:s3:::bucket/key",
	}
)

func setup(t *testing.T) (*mock_oswrapper.MockOS, *mock_oswrapper.MockFile, *mock_ioutilwrapper.MockIOUtil,
	*mock_credentials.MockManager, *mock_factory.MockS3ClientCreator, *mock_s3.MockS3Client, func()) {
	ctrl := gomock.NewController(t)

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockCredentialsManager := mock_credentials.NewMockManager(ctrl)
	mockS3ClientCreator := mock_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)

	return mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, ctrl.Finish
}

func newMockFirelensResource(firelensConfigType, networkMode string, lopOptions map[string]string, mockOS *mock_oswrapper.MockOS,
	mockIOUtil *mock_ioutilwrapper.MockIOUtil, mockCredentialsManager *mock_credentials.MockManager,
	mockS3ClientCreator *mock_factory.MockS3ClientCreator) *FirelensResource {
	return &FirelensResource{
		cluster:            testCluster,
		taskARN:            testTaskARN,
		taskDefinition:     testTaskDefinition,
		ec2InstanceID:      testEC2InstanceID,
		resourceDir:        testResourceDir,
		firelensConfigType: firelensConfigType,
		region:             testRegion,
		networkMode:        networkMode,
		containerToLogOptions: map[string]map[string]string{
			"container": lopOptions,
		},
		executionCredentialsID: testExecutionCredentialsID,
		credentialsManager:     mockCredentialsManager,
		os:                     mockOS,
		ioutil:                 mockIOUtil,
		s3ClientCreator:        mockS3ClientCreator,
	}
}

func TestParseOptions(t *testing.T) {
	firelensResource := FirelensResource{}
	err := firelensResource.parseOptions(testFirelensOptionsFile)
	assert.NoError(t, err)
	assert.Equal(t, true, firelensResource.ecsMetadataEnabled)
	assert.Equal(t, "file", firelensResource.externalConfigType)
	assert.Equal(t, "/tmp/dummy.conf", firelensResource.externalConfigValue)
}

func TestParseOptionsInvalidType(t *testing.T) {
	options := map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "invalid",
		"config-file-value":       "xxx",
	}
	firelensResource := FirelensResource{}
	assert.Error(t, firelensResource.parseOptions(options))
}

func TestParseOptionsNoValue(t *testing.T) {
	options := map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "file",
	}
	firelensResource := FirelensResource{}
	assert.Error(t, firelensResource.parseOptions(options))
}

func TestCreateFirelensResourceFluentdBridgeMode(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf"),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, firelensResource.Create())
}

func TestCreateFirelensResourceFluentdAWSVPCMode(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, awsvpcNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf"),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, firelensResource.Create())
}

func TestCreateFirelensResourceFluentdDefaultMode(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, "", testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf"),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, firelensResource.Create())
}

func TestCreateFirelensResourceFluentbit(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentbit, bridgeNetworkMode, testFluentbitOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf"),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, firelensResource.Create())
}

func TestCreateFirelensResourceInvalidType(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)
	firelensResource.firelensConfigType = "invalid"

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceCreateConfigDirError(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm).Return(errors.New("test error")),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceCreateSocketDirError(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm).Return(errors.New("test error")),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceGenerateConfigError(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)
	firelensResource.containerToLogOptions = map[string]map[string]string{
		"container": {
			"invalid": "invalid",
		},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceCreateTempFileError(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(nil, errors.New("test error")),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceWriteConfigFileError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes().Return(0, errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceChmodError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)).Return(errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceRenameError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf").Return(errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceWithS3Config(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	err := firelensResource.parseOptions(testFirelensOptionsS3)
	require.NoError(t, err)

	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     "id",
			SecretAccessKey: "key",
		},
	}
	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockCredentialsManager.EXPECT().GetTaskCredentials(testExecutionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ClientForBucket("bucket", testRegion, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		// write external config file downloaded from s3
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Do(
			func(ctx aws.Context, w io.WriterAt, input *s3.GetObjectInput) {
				assert.Equal(t, "bucket", aws.StringValue(input.Bucket))
				assert.Equal(t, "key", aws.StringValue(input.Key))
			}).Return(int64(0), nil),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/external.conf"),
		mockFile.EXPECT().Close(),

		// write main config file
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)),
		mockFile.EXPECT().Sync(),
		mockFile.EXPECT().Name().Return(testTempFile),
		mockOS.EXPECT().Rename(testTempFile, testResourceDir+"/config/fluent.conf"),
		mockFile.EXPECT().Close(),
	)

	assert.NoError(t, firelensResource.Create())
}

func TestCreateFirelensResourceWithS3ConfigMissingCredentials(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	err := firelensResource.parseOptions(testFirelensOptionsS3)
	require.NoError(t, err)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockCredentialsManager.EXPECT().GetTaskCredentials(testExecutionCredentialsID).Return(credentials.TaskIAMRoleCredentials{}, false),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceWithS3ConfigInvalidS3ARN(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	err := firelensResource.parseOptions(testFirelensOptionsS3)
	require.NoError(t, err)
	firelensResource.externalConfigValue = "arn:s3:::xxx"

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockCredentialsManager.EXPECT().GetTaskCredentials(testExecutionCredentialsID).Return(credentials.TaskIAMRoleCredentials{}, true),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCreateFirelensResourceWithS3ConfigDownloadFailure(t *testing.T) {
	mockOS, mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	err := firelensResource.parseOptions(testFirelensOptionsS3)
	require.NoError(t, err)

	creds := credentials.TaskIAMRoleCredentials{
		ARN: "arn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     "id",
			SecretAccessKey: "key",
		},
	}
	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockCredentialsManager.EXPECT().GetTaskCredentials(testExecutionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ClientForBucket("bucket", testRegion, creds.IAMRoleCredentials).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, firelensResource.Create())
	assert.NotEmpty(t, firelensResource.terminalReason)
}

func TestCleanupFirelensResource(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	mockOS.EXPECT().RemoveAll(testResourceDir)

	assert.NoError(t, firelensResource.Cleanup())
}

func TestCleanupFirelensResourceError(t *testing.T) {
	mockOS, _, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, mockOS, mockIOUtil,
		mockCredentialsManager, mockS3ClientCreator)

	mockOS.EXPECT().RemoveAll(testResourceDir).Return(errors.New("test error"))

	assert.Error(t, firelensResource.Cleanup())
}

func TestInitializeFirelensResource(t *testing.T) {
	_, _, _, mockCredentialsManager, _, _, done := setup(t)
	defer done()

	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, nil, nil,
		nil, nil)
	firelensResource.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: mockCredentialsManager,
		},
	}, status.TaskRunning, status.TaskRunning)

	assert.NotNil(t, firelensResource.statusToTransitions)
	assert.Equal(t, 1, len(firelensResource.statusToTransitions))
	assert.NotNil(t, firelensResource.os)
	assert.NotNil(t, firelensResource.ioutil)
	assert.NotNil(t, firelensResource.s3ClientCreator)
	assert.NotNil(t, firelensResource.credentialsManager)
}

func TestSetKnownStatus(t *testing.T) {
	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, nil, nil,
		nil, nil)
	firelensResource.appliedStatusUnsafe = resourcestatus.ResourceStatus(FirelensCreated)

	firelensResource.SetKnownStatus(resourcestatus.ResourceStatus(FirelensCreated))
	assert.Equal(t, resourcestatus.ResourceStatus(FirelensCreated), firelensResource.knownStatusUnsafe)
	assert.Equal(t, resourcestatus.ResourceStatus(FirelensStatusNone), firelensResource.appliedStatusUnsafe)
}

func TestSetKnownStatusNoAppliedStatusUpdate(t *testing.T) {
	firelensResource := newMockFirelensResource(FirelensConfigTypeFluentd, bridgeNetworkMode, testFluentdOptions, nil, nil,
		nil, nil)
	firelensResource.appliedStatusUnsafe = resourcestatus.ResourceStatus(FirelensCreated)

	firelensResource.SetKnownStatus(resourcestatus.ResourceStatus(FirelensStatusNone))
	assert.Equal(t, resourcestatus.ResourceStatus(FirelensStatusNone), firelensResource.knownStatusUnsafe)
	assert.Equal(t, resourcestatus.ResourceStatus(FirelensCreated), firelensResource.appliedStatusUnsafe)
}
