// +build linux,unit
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

package logrouter

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
)

const (
	testCluster         = "mycluster"
	testTaskARN         = "arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a"
	testTaskDefinition  = "taskdefinition:1"
	testEC2InstanceID   = "i-123456789a"
	testDataDir         = "testdatadir"
	testResourceDir     = "testresourcedir"
	testTerminalResason = "testterminalreason"
	testTempFile        = "testtempfile"
)

func setup(t *testing.T) (*mock_oswrapper.MockOS, *mock_oswrapper.MockFile, *mock_ioutilwrapper.MockIOUtil, func()) {
	ctrl := gomock.NewController(t)

	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)

	return mockOS, mockFile, mockIOUtil, ctrl.Finish
}

func newMockLogRouterResource(logRouterType string, logRouterOptions map[string]string, mockOS *mock_oswrapper.MockOS,
	mockIOUtil *mock_ioutilwrapper.MockIOUtil) *LogRouterResource {
	return &LogRouterResource{
		cluster:        testCluster,
		taskARN:        testTaskARN,
		taskDefinition: testTaskDefinition,
		ec2InstanceID:  testEC2InstanceID,
		resourceDir:    testResourceDir,
		logRouterType:  logRouterType,
		containerToLogOptions: map[string]map[string]string{
			"container": logRouterOptions,
		},
		os:     mockOS,
		ioutil: mockIOUtil,
	}
}

func TestCreateLogRouterResourceFluentd(t *testing.T) {
	mockOS, mockFile, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

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

	assert.NoError(t, logRouterResource.Create())
}

func TestCreateLogRouterResourceFluentbit(t *testing.T) {
	mockOS, mockFile, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentbit, testFluentbitOptions, mockOS, mockIOUtil)

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

	assert.NoError(t, logRouterResource.Create())
}

func TestCreateLogRouterResourceInvalidType(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)
	logRouterResource.logRouterType = "invalid"

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceCreateConfigDirError(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm).Return(errors.New("test error")),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceCreateSocketDirError(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm).Return(errors.New("test error")),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceGenerateConfigError(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)
	logRouterResource.containerToLogOptions = map[string]map[string]string{
		"container": {},
	}

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceCreateTempFileError(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(nil, errors.New("test error")),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceWriteConfigFileError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes().Return(0, errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceChmodError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(testResourceDir+"/config", os.ModePerm),
		mockOS.EXPECT().MkdirAll(testResourceDir+"/socket", os.ModePerm),
		mockIOUtil.EXPECT().TempFile(testResourceDir, tempFile).Return(mockFile, nil),
		mockFile.EXPECT().Write(gomock.Any()).AnyTimes(),
		mockFile.EXPECT().Chmod(os.FileMode(configFilePerm)).Return(errors.New("test error")),
		mockFile.EXPECT().Close(),
	)

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCreateLogRouterResourceRenameError(t *testing.T) {
	mockOS, mockFile, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

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

	assert.Error(t, logRouterResource.Create())
	assert.NotEmpty(t, logRouterResource.terminalReason)
}

func TestCleanupLogRouterResource(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	mockOS.EXPECT().RemoveAll(testResourceDir)

	assert.NoError(t, logRouterResource.Cleanup())
}

func TestCleanupLogRouterResourceError(t *testing.T) {
	mockOS, _, mockIOUtil, done := setup(t)
	defer done()

	logRouterResource := newMockLogRouterResource(LogRouterTypeFluentd, testFluentdOptions, mockOS, mockIOUtil)

	mockOS.EXPECT().RemoveAll(testResourceDir).Return(errors.New("test error"))

	assert.Error(t, logRouterResource.Cleanup())
}
