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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks/s3manager"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_bufio "github.com/aws/amazon-ecs-agent/agent/utils/bufiowrapper/mocks"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/aws-sdk-go-v2/aws"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

	// s3ARNFormat builds an environmentFile S3 ARN from a bucket and key.
	s3ARNFormat = "arn:aws:s3:::%s/%s"
)

var testIPCompatibility = ipcompatibility.NewIPCompatibility(true, true)

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
	mockIOUtil *mock_ioutilwrapper.MockIOUtil,
	ipCompatibility ipcompatibility.IPCompatibility) *EnvironmentFileResource {
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
		ipCompatibility:        ipCompatibility,
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
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	testConfig := &config.Config{InstanceIPCompatibility: testIPCompatibility}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, nil, nil, testIPCompatibility)
	envfileResource.Initialize(
		testConfig,
		&taskresource.ResourceFields{
			ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
				CredentialsManager: mockCredentialsManager,
			},
		}, status.TaskRunning, status.TaskRunning)

	assert.NotNil(t, envfileResource.statusToTransitions)
	assert.Equal(t, 1, len(envfileResource.statusToTransitions))
	assert.NotNil(t, envfileResource.credentialsManager)
	assert.NotNil(t, envfileResource.s3ClientCreator)
	assert.NotNil(t, envfileResource.ioutil)
	assert.NotNil(t, testIPCompatibility, envfileResource.ipCompatibility)
}

func TestCreateWithEnvVarFile(t *testing.T) {
	mockFile, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
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
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().Download(
			gomock.Any(), mockFile, gomock.Any(), gomock.Any(),
		).Do(
			func(ctx context.Context, w io.WriterAt, input *s3.GetObjectInput, options ...func(*s3manager.Downloader)) {
				assert.Equal(t, s3Bucket, aws.ToString(input.Bucket))
				assert.Equal(t, s3Key, aws.ToString(input.Key))
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

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
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
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().Download(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), errors.New("error response")),
	)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "error response")
}

func TestCreateUnableToCreateTmpFile(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()
	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	gomock.InOrder(
		mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil),
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
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
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
		mockS3ClientCreator.EXPECT().NewS3ManagerClient(s3Bucket, region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil),
		mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mockFile, nil),
		mockS3Client.EXPECT().Download(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), nil),
	)

	assert.Error(t, envfileResource.Create())
	assert.NotEmpty(t, envfileResource.terminalReasonUnsafe)
	assert.Contains(t, envfileResource.GetTerminalReason(), "error response")
}

func TestEnvFileCleanupSuccess(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)

	assert.NoError(t, envfileResource.Cleanup())
}

func TestEnvFileCleanupResourceDirRemoveFail(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)

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
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil, testIPCompatibility)
	envfileResource.bufio = mockBufio

	envfileContentLine1 := "key1=value"
	envFileContentLine2 := "key2=val1=val2"
	envFileContentLine3 := "key3="

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
		mockScanner.EXPECT().Scan().Return(true),
		mockScanner.EXPECT().Text().Return(envFileContentLine3),
		mockScanner.EXPECT().Scan().Return(false),
		mockScanner.EXPECT().Err().Return(nil),
	)

	envVarsList, err := envfileResource.ReadEnvVarsFromEnvfiles()

	assert.Nil(t, err)
	assert.Equal(t, 1, len(envVarsList))
	assert.Equal(t, "value", envVarsList[0]["key1"])
	assert.Equal(t, "val1=val2", envVarsList[0]["key2"])
	key3Value, ok := envVarsList[0]["key3"]
	assert.True(t, ok)
	assert.Equal(t, "", key3Value)
}

func TestReadEnvVarsCommentFromEnvfiles(t *testing.T) {
	mockFile, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	ctrl := gomock.NewController(t)
	mockBufio := mock_bufio.NewMockBufio(ctrl)
	mockScanner := mock_bufio.NewMockScanner(ctrl)

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil, testIPCompatibility)
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
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil, testIPCompatibility)
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

// TestCreateRejectsNonLocalKey verifies that an S3 key that would resolve
// outside the task's envfile resource directory is rejected before any file is
// written.
func TestCreateRejectsNonLocalKey(t *testing.T) {
	nonLocalKeys := []struct {
		name string
		arn  string
	}{
		{"parent reference", fmt.Sprintf(s3ARNFormat, s3Bucket, "../../../../config.env")},
		{"deep parent reference", fmt.Sprintf(s3ARNFormat, s3Bucket, "a/b/c/../../../../../../config.env")},
	}
	// A backslash is a path separator on Windows but an ordinary character on
	// other platforms, so a backslash-padded key only resolves outside the
	// resource directory off Windows. Validate that case where it applies.
	if runtime.GOOS != "windows" {
		nonLocalKeys = append(nonLocalKeys, struct {
			name string
			arn  string
		}{"backslash padded reference", fmt.Sprintf(s3ARNFormat, s3Bucket, `a\b\c/../../../../config.env`)})
	}

	for _, tc := range nonLocalKeys {
		t.Run(tc.name, func(t *testing.T) {
			_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
			defer done()

			envfiles := []container.EnvironmentFile{
				sampleEnvironmentFile(tc.arn, "s3"),
			}

			envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
			creds := credentials.TaskIAMRoleCredentials{
				ARN: iamRoleARN,
				IAMRoleCredentials: credentials.IAMRoleCredentials{
					AccessKeyID:     accessKeyId,
					SecretAccessKey: secretAccessKey,
				},
			}

			// The key must be rejected before any temp file is created or any
			// S3 download happens, so TempFile/Download are never expected.
			mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
			mockS3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil).AnyTimes()

			assert.Error(t, envfileResource.Create())
			assert.Contains(t, envfileResource.GetTerminalReason(), "escapes envfile resource directory")
		})
	}
}

// TestCreateRejectsNonLocalEnvfileInMultiFileSet covers a task definition that
// lists two envfiles downloading concurrently: one with a normal key and one with
// a key that resolves outside resourceDir. The first key's directory creation must
// not let the second one's rename land outside resourceDir; the confinement check
// rejects the non-local key regardless of the other file succeeding.
func TestCreateRejectsNonLocalEnvfileInMultiFileSet(t *testing.T) {
	_, mockIOUtil, mockCredentialsManager, mockS3ClientCreator, mockS3Client, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, "config/app.env"), "s3"),
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, "config/a/b/c/../../../../../../escaped.env"), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, mockCredentialsManager, mockS3ClientCreator, mockIOUtil, testIPCompatibility)
	creds := credentials.TaskIAMRoleCredentials{
		ARN: iamRoleARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
	}

	// Capture every rename target so we can prove the non-local file is never
	// moved into place. Downloads run concurrently, so guard the slice with a mutex.
	var renameMu sync.Mutex
	var renameTargets []string
	rename = func(oldpath, newpath string) error {
		renameMu.Lock()
		renameTargets = append(renameTargets, newpath)
		renameMu.Unlock()
		return nil
	}
	defer func() {
		rename = os.Rename
	}()

	// Both envfiles download concurrently. The in-bounds one may create a temp
	// file and download; the non-local one must be rejected by the check.
	mockCredentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
	mockS3ClientCreator.EXPECT().NewS3ManagerClient(gomock.Any(), region, creds.IAMRoleCredentials, testIPCompatibility).Return(mockS3Client, nil).AnyTimes()
	mockIOUtil.EXPECT().TempFile(resourceDir, gomock.Any()).Return(mock_oswrapper.NewMockFile(), nil).AnyTimes()
	mockS3Client.EXPECT().Download(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()

	err := envfileResource.Create()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes envfile resource directory")

	// The expectation is that the non-local envfile is never written, not just
	// that an error was returned. Assert no rename targeted that file and that
	// every rename that did happen stayed within resourceDir.
	renameMu.Lock()
	defer renameMu.Unlock()
	cleanBase := filepath.Clean(resourceDir) + string(os.PathSeparator)
	for _, target := range renameTargets {
		assert.NotContains(t, target, "escaped.env",
			"non-local envfile was renamed into place: %s", target)
		assert.True(t, strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanBase),
			"rename target %q resolved outside resourceDir %q", target, resourceDir)
	}
}

// TestReadEnvVarsRejectsNonLocalKey verifies the read path is also confined, so a
// key that resolves outside resourceDir cannot be used to read files off the data
// mount.
func TestReadEnvVarsRejectsNonLocalKey(t *testing.T) {
	_, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, "../../../../config.env"), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil, testIPCompatibility)

	_, err := envfileResource.ReadEnvVarsFromEnvfiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes envfile resource directory")
}

// TestVerifyEnvfileKeyLocal exercises the confinement helper directly. The
// bucket/key comes from the task definition; it must stay local to the resource
// directory once appended to it.
func TestVerifyEnvfileKeyLocal(t *testing.T) {
	type keyCase struct {
		name    string
		bucket  string
		key     string
		wantErr bool
	}
	cases := []keyCase{
		{"simple file", "bucket", "app.env", false},
		{"nested file", "bucket", "a/b/c.env", false},
		{"parent reference resolving back inside", "bucket", "a/../app.env", false},
		{"parent reference", "bucket", "../../../../config.env", true},
		{"deep parent reference", "bucket", "a/b/c/../../../../../../config.env", true},
	}
	// A backslash is a path separator on Windows but an ordinary character on
	// other platforms, so a backslash-padded key only resolves outside the
	// resource directory off Windows. Validate those cases where they apply.
	if runtime.GOOS != "windows" {
		cases = append(cases,
			keyCase{"backslash padded reference", "bucket", `a\b\c/../../../../config.env`, true},
			keyCase{"deep backslash padded reference", "bucket", `a\b\c\d\e\f\g\h/../../../../../../../var/lib/config.env`, true},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyEnvfileKeyLocal(tc.bucket, tc.key)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReadEnvVarsUnableToReadEnvfile(t *testing.T) {
	_, mockIOUtil, _, _, _, done := setup(t)
	defer done()

	envfiles := []container.EnvironmentFile{
		sampleEnvironmentFile(fmt.Sprintf(s3ARNFormat, s3Bucket, s3Key), "s3"),
	}

	envfileResource := newMockEnvfileResource(envfiles, nil, nil, mockIOUtil, testIPCompatibility)

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
