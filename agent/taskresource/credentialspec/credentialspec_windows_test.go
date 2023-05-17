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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks/s3manager"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mock_ssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN                        = "arn:aws:ecs:us-west-2:123456789012:task/12345-678901234-56789"
	ssmARNRoot                     = "arn:aws:ssm:us-west-2:123456789012:parameter/"
	executionCredentialsID         = "exec-creds-id"
	testTempFile                   = "testtempfile"
	credentialSpecResourceLocation = "C:\\ProgramData\\docker\\credentialspecs"
)

func mockRename() func() {
	rename = func(oldpath, newpath string) error {
		return nil
	}

	return func() {
		rename = os.Rename
	}
}

func TestClearCredentialSpecData(t *testing.T) {
	testCases := []struct {
		name                                           string
		credSpecMapData                                map[string]string
		finalCredSpecMapKeys                           []string
		osRemoveImplError                              error
		deleteTaskExecutionCredentialsRegKeysImplError error
		expectedErrorString                            string
		domainlessGMSATask                             bool
	}{
		{
			name: "credentialSpecHappyCase",
			credSpecMapData: map[string]string{
				"credentialspec:file://localfilePath.json":                                   "credentialspec=file://localfilePath.json",
				"credentialspec:arn:aws:s3:::bucket_name/gmsa-cred-spec":                     "credentialspec=file://s3_taskARN_fileName.json",
				"credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec": "credentialspec=file://ssm_taskARN_fileName.json",
			},
			osRemoveImplError:    nil,
			expectedErrorString:  "",
			finalCredSpecMapKeys: []string{"credentialspec:file://localfilePath.json"},
			domainlessGMSATask:   false,
		},
		{
			name: "credentialSpecErrCaseAndNoFailure",
			credSpecMapData: map[string]string{
				"credentialspec:file://localfilePath.json":                                   "credentialspec=file://localfilePath.json",
				"credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec": "credentialspec=file://ssm_taskARN_fileName.json",
			},
			osRemoveImplError:    errors.New("mock osRemoveImplError"),
			expectedErrorString:  "",
			finalCredSpecMapKeys: []string{"credentialspec:file://localfilePath.json"},
			domainlessGMSATask:   false,
		},
		{
			name: "credentialSpecDomainlessHappyCase",
			credSpecMapData: map[string]string{
				"credentialspec:file://localfilePath.json":                                             "credentialspec=file://localfilePath.json",
				"credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec":           "credentialspec=file://ssm_taskARN_fileName.json",
				"credentialspecdomainless:file://localfilePath.json":                                   "credentialspec=file://localfilePath.json",
				"credentialspecdomainless:arn:aws:s3:::bucket_name/gmsa-cred-spec":                     "credentialspec=file://s3_taskARN_fileName.json",
				"credentialspecdomainless:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec": "credentialspec=file://ssm_taskARN_fileName.json",
			},
			osRemoveImplError: nil,
			deleteTaskExecutionCredentialsRegKeysImplError: nil,
			expectedErrorString:                            "",
			finalCredSpecMapKeys:                           []string{"credentialspec:file://localfilePath.json"},
			domainlessGMSATask:                             true,
		},
		{
			name: "credentialSpecDomainlessErrCase",
			credSpecMapData: map[string]string{
				"credentialspec:file://localfilePath.json":                                             "credentialspec=file://localfilePath.json",
				"credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec":           "credentialspec=file://ssm_taskARN_fileName.json",
				"credentialspecdomainless:file://localfilePath.json":                                   "credentialspec=file://localfilePath.json",
				"credentialspecdomainless:arn:aws:s3:::bucket_name/gmsa-cred-spec":                     "credentialspec=file://s3_taskARN_fileName.json",
				"credentialspecdomainless:arn:aws:ssm:us-west-2:123456789012:parameter/gmsa-cred-spec": "credentialspec=file://ssm_taskARN_fileName.json",
			},
			osRemoveImplError: nil,
			deleteTaskExecutionCredentialsRegKeysImplError: errors.New("mock deleteTaskExecutionCredentialsRegKeysImplError"),
			expectedErrorString:                            "mock deleteTaskExecutionCredentialsRegKeysImplError",
			finalCredSpecMapKeys:                           []string{"credentialspec:file://localfilePath.json"},
			domainlessGMSATask:                             true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			osRemoveImpl = func(name string) error {
				return tc.osRemoveImplError
			}
			defer func() {
				osRemoveImpl = os.Remove
			}()

			deleteTaskExecutionCredentialsRegKeysImpl = func(taskArn string) error {
				return tc.deleteTaskExecutionCredentialsRegKeysImplError
			}
			defer func() {
				deleteTaskExecutionCredentialsRegKeysImpl = deleteTaskExecutionCredentialsRegKeys
			}()

			credSpecMapData := tc.credSpecMapData

			credspecRes := &CredentialSpecResource{
				CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
					CredSpecMap: credSpecMapData,
				},
				credentialSpecResourceLocation: credentialSpecResourceLocation,
				isDomainlessGMSATask:           tc.domainlessGMSATask,
			}

			err := credspecRes.Cleanup()
			if tc.expectedErrorString != "" {
				assert.EqualError(t, err, tc.expectedErrorString)
			} else {
				assert.NoError(t, err)
			}

			if tc.finalCredSpecMapKeys != nil {
				assert.Equal(t, len(tc.finalCredSpecMapKeys), len(credspecRes.CredSpecMap))

				for _, expectedKey := range tc.finalCredSpecMapKeys {
					_, ok := credspecRes.CredSpecMap[expectedKey]
					assert.True(t, ok)
				}
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	credspecRes := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:   resourcestatus.ResourceCreated,
			desiredStatusUnsafe: resourcestatus.ResourceCreated,
		},
	}
	credspecRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			taskARN:                    taskARN,
			executionCredentialsID:     executionCredentialsID,
			createdAt:                  time.Now(),
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			credentialSpecContainerMap: credentialSpecContainerMap,
			CredSpecMap:                credSpecMap,
		},
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
	testCases := []struct {
		name                                                          string
		fileCredentialSpec                                            string
		expectedFileCredentialSpec                                    string
		taskARN                                                       string
		readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath string
		readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath   string
		readWriteDomainlessCredentialSpecImplError                    error
		expectedErrorString                                           string
	}{
		{
			name:                       "credentialSpecHappyCase",
			fileCredentialSpec:         "credentialspec:file://test.json",
			expectedFileCredentialSpec: "credentialspec=file://test.json",
			readWriteDomainlessCredentialSpecImplError: nil,
			expectedErrorString:                        "",
		},
		{
			name:                       "credentialSpecNoErrCase",
			fileCredentialSpec:         "credentialspec:file://test.json",
			expectedFileCredentialSpec: "credentialspec=file://test.json",
			readWriteDomainlessCredentialSpecImplError: errors.New("mocked readWriteDomainlessCredentialSpecImplError"),
			expectedErrorString:                        "",
		},
		{
			name:                       "credentialSpecDir",
			fileCredentialSpec:         "credentialspec:file://relativeDir\\test.json",
			expectedFileCredentialSpec: "credentialspec=file://relativeDir\\test.json",
			readWriteDomainlessCredentialSpecImplError: nil,
			expectedErrorString:                        "",
		},
		{
			name:                       "credentialSpecDomainlessHappyCase",
			fileCredentialSpec:         "credentialspecdomainless:file://test.json",
			expectedFileCredentialSpec: "credentialspec=file://24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			taskARN:                    "arn:aws:ecs:us-west-2:123456789012:task/windows-domainless-gmsa-cluster/24392c5d89b9457d80b7ad1a3638ddba",
			readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath: credentialSpecResourceLocation + "\\test.json",
			readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath:   credentialSpecResourceLocation + "\\24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			readWriteDomainlessCredentialSpecImplError:                    nil,
			expectedErrorString: "",
		},
		{
			name:                       "credentialSpecDomainlessErrCase",
			fileCredentialSpec:         "credentialspecdomainless:file://test.json",
			expectedFileCredentialSpec: "credentialspec=file://24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			taskARN:                    "arn:aws:ecs:us-west-2:123456789012:task/windows-domainless-gmsa-cluster/24392c5d89b9457d80b7ad1a3638ddba",
			readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath: credentialSpecResourceLocation + "\\test.json",
			readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath:   credentialSpecResourceLocation + "\\24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			readWriteDomainlessCredentialSpecImplError:                    errors.New("mocked readWriteDomainlessCredentialSpecImplError"),
			expectedErrorString: "mocked readWriteDomainlessCredentialSpecImplError",
		},
		{
			name:                       "credentialSpecDomainlessRelativeDir",
			fileCredentialSpec:         "credentialspecdomainless:file://relativeDir\\test.json",
			expectedFileCredentialSpec: "credentialspec=file://relativeDir\\24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			taskARN:                    "arn:aws:ecs:us-west-2:123456789012:task/windows-domainless-gmsa-cluster/24392c5d89b9457d80b7ad1a3638ddba",
			readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath: credentialSpecResourceLocation + "\\relativeDir\\test.json",
			readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath:   credentialSpecResourceLocation + "\\relativeDir\\24392c5d89b9457d80b7ad1a3638ddba_webapp_test.json",
			readWriteDomainlessCredentialSpecImplError:                    nil,
			expectedErrorString: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			readWriteDomainlessCredentialSpecImpl = func(filePath, outFilePath, taskARN string) error {
				if tc.readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath != filePath {
					return errors.New(fmt.Sprintf("Expected input filePath to be %v, instead got %v.", tc.readWriteDomainlessCredentialSpecImplExpectedOriginalFilePath, filePath))
				}

				if tc.readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath != outFilePath {
					return errors.New(fmt.Sprintf("Expected input outFilePath to be %v, instead got %v.", tc.readWriteDomainlessCredentialSpecImplExpectedCopiedFilePath, outFilePath))
				}

				if tc.taskARN != taskARN {
					return errors.New(fmt.Sprintf("Expected input taskARN to be %v, instead got %v.", tc.taskARN, taskARN))
				}
				return tc.readWriteDomainlessCredentialSpecImplError
			}

			defer func() {
				readWriteDomainlessCredentialSpecImpl = readWriteDomainlessCredentialSpec
			}()

			credentialSpecContainerMap := map[string]string{tc.fileCredentialSpec: "webapp"}
			cs := &CredentialSpecResource{
				CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
					credentialSpecContainerMap: credentialSpecContainerMap,
					CredSpecMap:                map[string]string{},
					taskARN:                    tc.taskARN,
				},
				credentialSpecResourceLocation: credentialSpecResourceLocation,
			}

			err := cs.handleCredentialspecFile(tc.fileCredentialSpec)
			if tc.expectedErrorString != "" {
				assert.EqualError(t, err, tc.expectedErrorString)
			} else {
				assert.NoError(t, err)

				targetCredentialSpecFile, err := cs.GetTargetMapping(tc.fileCredentialSpec)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedFileCredentialSpec, targetCredentialSpecFile)
			}
		})
	}
}

func TestHandleCredentialspecFileErr(t *testing.T) {
	fileCredentialSpec := "credentialspec:invalid-file://test.json"
	credentialSpecContainerMap := map[string]string{fileCredentialSpec: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
	}

	err := cs.handleCredentialspecFile(fileCredentialSpec)
	assert.Error(t, err)
}

func TestHandleSSMCredentialspecFile(t *testing.T) {
	testCases := []struct {
		name                                         string
		ssmParameterName                             string
		credentialSpecPrefix                         string
		expectedLocalFilePath                        string
		expectedtaskArn                              string
		handleNonFileDomainlessGMSACredSpecImplError error
		expectedErrorString                          string
	}{
		{
			name:                 "credentialSpecHappyCase",
			credentialSpecPrefix: "credentialspec:",
			ssmParameterName:     "test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                 "credentialSpecRelativeParam",
			credentialSpecPrefix: "credentialspec:",
			ssmParameterName:     "x/y/test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                 "credentialSpecDomainlessHappyCase",
			credentialSpecPrefix: "credentialspecdomainless:",
			ssmParameterName:     "test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                 "credentialSpecDomainlessRelativeParam",
			credentialSpecPrefix: "credentialspecdomainless:",
			ssmParameterName:     "test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                 "credentialSpecErr",
			credentialSpecPrefix: "credentialspec:",
			ssmParameterName:     "test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: errors.New("mocked handleNonFileDomainlessGMSACredSpecImplError"),
			expectedErrorString:                          "mocked handleNonFileDomainlessGMSACredSpecImplError",
		},
		{
			name:                 "credentialSpecDomainlessErr",
			credentialSpecPrefix: "credentialspecdomainless:",
			ssmParameterName:     "test",
			expectedtaskArn:      taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: errors.New("mocked handleNonFileDomainlessGMSACredSpecImplError"),
			expectedErrorString:                          "mocked handleNonFileDomainlessGMSACredSpecImplError",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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

			credentialSpecSSMARN := ssmARNRoot + tc.ssmParameterName
			ssmCredentialSpec := tc.credentialSpecPrefix + credentialSpecSSMARN
			customCredSpecFileName := fmt.Sprintf("%s%s%s", "12345-678901234-56789", containerName, credentialSpecSSMARN)
			hasher := sha256.New()
			hasher.Write([]byte(customCredSpecFileName))
			customCredSpecFileName = fmt.Sprintf("%x", hasher.Sum(nil))
			expectedFileCredentialSpec := fmt.Sprintf("credentialspec=file://%s", customCredSpecFileName)

			credentialSpecContainerMap := map[string]string{
				ssmCredentialSpec: containerName,
			}

			cs := &CredentialSpecResource{
				CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
					knownStatusUnsafe:          resourcestatus.ResourceCreated,
					desiredStatusUnsafe:        resourcestatus.ResourceCreated,
					CredSpecMap:                map[string]string{},
					taskARN:                    taskARN,
					credentialSpecContainerMap: credentialSpecContainerMap,
				},
				credentialSpecResourceLocation: credentialSpecResourceLocation,
				ioutil:                         mockIO,
			}
			cs.Initialize(&taskresource.ResourceFields{
				ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
					SSMClientCreator:   ssmClientCreator,
					CredentialsManager: credentialsManager,
					S3ClientCreator:    s3ClientCreator,
				},
			}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

			testData := "test-cred-spec-data"
			ssmClientOutput := &ssm.GetParametersOutput{
				InvalidParameters: []*string{},
				Parameters: []*ssm.Parameter{
					&ssm.Parameter{
						Name:  aws.String(tc.ssmParameterName),
						Value: aws.String(testData),
					},
				},
			}

			gomock.InOrder(
				ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
				mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
				mockIO.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)

			handleNonFileDomainlessGMSACredSpecImpl = func(originalCredSpec, localCredSpecFilePath, taskARN string) error {
				if credentialSpecResourceLocation+"\\"+customCredSpecFileName != localCredSpecFilePath {
					return errors.New(fmt.Sprintf("Expected input localCredSpecFilePath to be %v, instead got %v.", credentialSpecResourceLocation+"\\"+customCredSpecFileName, localCredSpecFilePath))
				}

				if tc.expectedtaskArn != taskARN {
					return errors.New(fmt.Sprintf("Expected input taskARN to be %v, instead got %v.", tc.expectedtaskArn, taskARN))
				}

				return tc.handleNonFileDomainlessGMSACredSpecImplError
			}

			defer func() { handleNonFileDomainlessGMSACredSpecImpl = handleNonFileDomainlessGMSACredSpec }()

			err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
			if tc.expectedErrorString != "" {
				assert.NotNil(t, err)
				assert.EqualError(t, err, tc.expectedErrorString)
			} else {
				assert.NoError(t, err)

				targetCredentialSpecFile, err := cs.GetTargetMapping(ssmCredentialSpec)
				assert.NoError(t, err)
				assert.Equal(t, expectedFileCredentialSpec, targetCredentialSpecFile)
			}
		})
	}

}

func TestHandleSSMCredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	var termReason string
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			terminalReason: termReason,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ioutil: mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"
	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	iamCredentials := credentials.IAMRoleCredentials{}

	err := cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials)
	assert.Error(t, err)
}

func TestHandleS3CredentialspecFile(t *testing.T) {
	testCases := []struct {
		name                                         string
		s3ObjectName                                 string
		credentialSpecPrefix                         string
		expectedLocalFilePath                        string
		expectedFileCredentialSpecFile               string
		expectedtaskArn                              string
		handleNonFileDomainlessGMSACredSpecImplError error
		expectedErrorString                          string
	}{
		{
			name:                           "credentialSpecHappyCase",
			credentialSpecPrefix:           "credentialspec:",
			s3ObjectName:                   "test",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                           "credentialSpecRelativeParam",
			credentialSpecPrefix:           "credentialspec:",
			s3ObjectName:                   "test/test2/test3",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test3",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                           "credentialSpecDomainlessHappyCase",
			credentialSpecPrefix:           "credentialspecdomainless:",
			s3ObjectName:                   "test",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                           "credentialSpecDomainlessRelativeParam",
			credentialSpecPrefix:           "credentialspecdomainless:",
			s3ObjectName:                   "test/test2/test3",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test3",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: nil,
			expectedErrorString:                          "",
		},
		{
			name:                           "credentialSpecErrCase",
			credentialSpecPrefix:           "credentialspec:",
			s3ObjectName:                   "test",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: errors.New("mocked handleNonFileDomainlessGMSACredSpecImplError"),
			expectedErrorString:                          "mocked handleNonFileDomainlessGMSACredSpecImplError",
		},
		{
			name:                           "credentialSpecDomainlessErrCase",
			credentialSpecPrefix:           "credentialspecdomainless:",
			s3ObjectName:                   "test",
			expectedFileCredentialSpecFile: "s3_12345-678901234-56789_test",
			expectedtaskArn:                taskARN,
			handleNonFileDomainlessGMSACredSpecImplError: errors.New("mocked handleNonFileDomainlessGMSACredSpecImplError"),
			expectedErrorString:                          "mocked handleNonFileDomainlessGMSACredSpecImplError",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
			credentialSpecS3ARN := "arn:aws:s3:::bucket_name/" + tc.s3ObjectName
			s3CredentialSpec := tc.credentialSpecPrefix + credentialSpecS3ARN

			credentialSpecContainerMap := map[string]string{s3CredentialSpec: "webapp"}

			cs := &CredentialSpecResource{
				CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
					knownStatusUnsafe:          resourcestatus.ResourceCreated,
					desiredStatusUnsafe:        resourcestatus.ResourceCreated,
					CredSpecMap:                map[string]string{},
					taskARN:                    taskARN,
					credentialSpecContainerMap: credentialSpecContainerMap,
				},
				credentialSpecResourceLocation: credentialSpecResourceLocation,
				ioutil:                         mockIO,
			}
			cs.Initialize(&taskresource.ResourceFields{
				ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
					SSMClientCreator:   ssmClientCreator,
					CredentialsManager: credentialsManager,
					S3ClientCreator:    s3ClientCreator,
				},
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

			handleNonFileDomainlessGMSACredSpecImpl = func(originalCredSpec, localCredSpecFilePath, taskARN string) error {
				if fmt.Sprintf("%sarn:aws:s3:::bucket_name/%s", tc.credentialSpecPrefix, tc.s3ObjectName) != originalCredSpec {
					return errors.New(fmt.Sprintf("Expected input originalCredSpec to be %sarn:aws:s3:::bucket_name/%s, instead got %s.", tc.credentialSpecPrefix, tc.s3ObjectName, originalCredSpec))
				}

				if credentialSpecResourceLocation+"\\"+tc.expectedFileCredentialSpecFile != localCredSpecFilePath {
					return errors.New(fmt.Sprintf("Expected input localCredSpecFilePath to be %s, instead got %s.", credentialSpecResourceLocation+"\\"+tc.expectedFileCredentialSpecFile, localCredSpecFilePath))
				}

				if tc.expectedtaskArn != taskARN {
					return errors.New(fmt.Sprintf("Expected input taskARN to be %s, instead got %s.", tc.expectedtaskArn, taskARN))
				}

				return tc.handleNonFileDomainlessGMSACredSpecImplError
			}

			defer func() { handleNonFileDomainlessGMSACredSpecImpl = handleNonFileDomainlessGMSACredSpec }()

			err := cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials)
			if tc.expectedErrorString != "" {
				assert.NotNil(t, err)
				assert.EqualError(t, err, tc.expectedErrorString)
			} else {
				assert.NoError(t, err)

				targetCredentialSpecFile, err := cs.GetTargetMapping(s3CredentialSpec)
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("credentialspec=file://%s", tc.expectedFileCredentialSpecFile), targetCredentialSpecFile)
			}
		})
	}
}

func TestHandleS3CredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecS3ARN := "arn:aws:/test"
	s3CredentialSpec := "credentialspec:arn:/test"

	var termReason string
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			terminalReason: termReason,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			terminalReason: termReason,
		},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ioutil: mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ioutil: mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ioutil: mockIO,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
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
	testCases := []struct {
		name                                              string
		fileCredentialSpec                                string
		setTaskExecutionCredentialsRegKeysImplReturnValue error
		readWriteDomainlessCredentialSpecImplReturnValue  error
		expectedCreateError                               string
	}{
		{
			name:               "fileCredSpecNoErr",
			fileCredentialSpec: "credentialspec:file://test.json",
			setTaskExecutionCredentialsRegKeysImplReturnValue: nil,
			expectedCreateError: "",
		},
		{
			name:               "fileCredSpecNoErr2",
			fileCredentialSpec: "credentialspec:file://test.json",
			setTaskExecutionCredentialsRegKeysImplReturnValue: errors.New("mock setTaskExecutionCredentialsRegKeysImpl err"),
			expectedCreateError: "",
		},
		{
			name:               "domainlessfileCredSpecNoErr",
			fileCredentialSpec: "credentialspecdomainless:file://test.json",
			setTaskExecutionCredentialsRegKeysImplReturnValue: nil,
			expectedCreateError: "",
		},
		{
			name:               "domainlessfileCredSpecErr",
			fileCredentialSpec: "credentialspecdomainless:file://test.json",
			setTaskExecutionCredentialsRegKeysImplReturnValue: errors.New("mock setTaskExecutionCredentialsRegKeysImpl err"),
			expectedCreateError: "mock setTaskExecutionCredentialsRegKeysImpl err",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			readWriteDomainlessCredentialSpecImpl = func(filePath, outFilePath, taskARN string) error {
				return tc.readWriteDomainlessCredentialSpecImplReturnValue
			}
			defer func() {
				readWriteDomainlessCredentialSpecImpl = readWriteDomainlessCredentialSpec
			}()

			setTaskExecutionCredentialsRegKeysImpl = func(taskCredentials credentials.IAMRoleCredentials, taskArn string) error {
				return tc.setTaskExecutionCredentialsRegKeysImplReturnValue
			}
			defer func() {
				readWriteDomainlessCredentialSpecImpl = readWriteDomainlessCredentialSpec
			}()

			credentialsManager := mock_credentials.NewMockManager(ctrl)
			credentialSpecContainerMap := map[string]string{tc.fileCredentialSpec: "webapp"}

			cs := &CredentialSpecResource{
				CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
					credentialsManager:         credentialsManager,
					CredSpecMap:                map[string]string{},
					credentialSpecContainerMap: credentialSpecContainerMap,
					taskARN:                    "arn:aws:ecs:us-west-2:123456789012:task/windows-domainless-gmsa-cluster/24392c5d89b9457d80b7ad1a3638ddba",
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
			)
			err := cs.Create()
			if tc.expectedCreateError != "" {
				assert.EqualError(t, err, tc.expectedCreateError)
			}

		})
	}
}

func TestGetName(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	assert.Equal(t, ResourceName, cs.GetName())
}

func TestGetTargetMapping(t *testing.T) {
	inputCredSpec, outputCredSpec := "credentialspec:file://test.json", "credentialspec=file://test.json"
	credSpecMapData := map[string]string{
		inputCredSpec: outputCredSpec,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
	}

	targetCredSpec, err := cs.GetTargetMapping(inputCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, outputCredSpec, targetCredSpec)
}

func TestGetTargetMappingErr(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: map[string]string{},
		},
	}

	targetCredSpec, err := cs.GetTargetMapping("testcredspec")
	assert.Error(t, err)
	assert.Empty(t, targetCredSpec)
}

func TestSetTaskExecutionCredentialsRegKeysErr(t *testing.T) {
	var iamCredentials credentials.IAMRoleCredentials
	err := SetTaskExecutionCredentialsRegKeys(iamCredentials, "12345678")
	assert.EqualError(t, err, "Unable to find execution role credentials while setting registry key for task 12345678")
}

func TestHandleNonFileDomainlessGMSACredSpec(t *testing.T) {
	testCases := []struct {
		name                                                   string
		originalCredSpec                                       string
		localCredSpecFilePath                                  string
		taskARN                                                string
		readWriteDomainlessCredentialSpecImplReturnValue       error
		expectedHandleNonFileDomainlessGMSACredSpecErrorString string
	}{
		{
			name:                  "domainlessCredSpecErr",
			originalCredSpec:      "credentialspecdomainless:arn:aws:ssm:us-west-2:123456789012:parameter/test",
			localCredSpecFilePath: "random_file",
			taskARN:               "12345678",
			readWriteDomainlessCredentialSpecImplReturnValue:       errors.New("mock readWriteDomainlessCredentialSpecImplReturnValue err"),
			expectedHandleNonFileDomainlessGMSACredSpecErrorString: "mock readWriteDomainlessCredentialSpecImplReturnValue err",
		},
		{
			name:                  "domainlessCredSpecNoErr",
			originalCredSpec:      "credentialspecdomainless:arn:aws:ssm:us-west-2:123456789012:parameter/test",
			localCredSpecFilePath: "random_file",
			taskARN:               "12345678",
			readWriteDomainlessCredentialSpecImplReturnValue:       nil,
			expectedHandleNonFileDomainlessGMSACredSpecErrorString: "",
		},
		{
			name:                  "nonDomainlessCredSpecEarlyExit",
			originalCredSpec:      "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test",
			localCredSpecFilePath: "random_file",
			taskARN:               "12345678",
			readWriteDomainlessCredentialSpecImplReturnValue:       errors.New("mock readWriteDomainlessCredentialSpecImplReturnValue err"),
			expectedHandleNonFileDomainlessGMSACredSpecErrorString: "",
		},
		{
			name:                  "nonDomainlessCredSpecEarlyExit2",
			originalCredSpec:      "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test",
			localCredSpecFilePath: "random_file",
			taskARN:               "12345678",
			readWriteDomainlessCredentialSpecImplReturnValue:       nil,
			expectedHandleNonFileDomainlessGMSACredSpecErrorString: "",
		},
	}

	for _, tc := range testCases {
		readWriteDomainlessCredentialSpecImpl = func(filePath, outFilePath, taskARN string) error {
			if filePath != outFilePath {
				return errors.New(fmt.Sprintf("Expected input filepath %s to be equal to input outFilePath %s", filePath, outFilePath))
			}
			if tc.localCredSpecFilePath != filePath {
				return errors.New(fmt.Sprintf("Expected input filepath to be %s, instead got %s", tc.localCredSpecFilePath, filePath))
			}
			return tc.readWriteDomainlessCredentialSpecImplReturnValue
		}

		t.Run(tc.name, func(t *testing.T) {
			err := handleNonFileDomainlessGMSACredSpec(tc.originalCredSpec, tc.localCredSpecFilePath, tc.taskARN)
			if tc.expectedHandleNonFileDomainlessGMSACredSpecErrorString != "" {
				assert.EqualError(t, err, tc.expectedHandleNonFileDomainlessGMSACredSpecErrorString)
			}

		})

		readWriteDomainlessCredentialSpecImpl = readWriteDomainlessCredentialSpec
	}
}

func TestReadWriteDomainlessCredentialSpecHappyCase(t *testing.T) {
	sampleFilePath := "random_file"
	sampleOutfilePath := "random_file_out"
	sampleTaskArn := "12345678"
	sampleCredSpec := map[string]interface{}{"ActiveDirectoryConfig": "sample_value"}
	readCredentialSpecImpl = func(filePath string) (map[string]interface{}, error) {
		if sampleFilePath != filePath {
			return nil, errors.New(fmt.Sprintf("Expected input filepath to be %s, instead got %s", sampleFilePath, filePath))
		}
		return sampleCredSpec, nil
	}

	defer func() {
		readCredentialSpecImpl = readCredentialSpec
	}()

	writeCredentialSpecImpl = func(credSpec map[string]interface{}, outFilePath string, taskARN string) error {
		sampleCredSpecBytes, _ := json.Marshal(sampleCredSpec)
		credSpecBytes, _ := json.Marshal(credSpec)
		if !bytes.Equal(sampleCredSpecBytes, credSpecBytes) {
			return errors.New(fmt.Sprintf("Expected input credSpec to be %v, instead got %v.", sampleCredSpec, credSpec))
		}
		if sampleOutfilePath != outFilePath {
			return errors.New(fmt.Sprintf("Expected input outFilePath to be %s, instead got %s", sampleOutfilePath, outFilePath))
		}
		if sampleTaskArn != taskARN {
			return errors.New(fmt.Sprintf("Expected input taskARN to be %s, instead got %s", sampleTaskArn, taskARN))
		}

		return nil
	}

	defer func() {
		writeCredentialSpecImpl = writeCredentialSpec
	}()

	err := readWriteDomainlessCredentialSpec(sampleFilePath, sampleOutfilePath, sampleTaskArn)
	assert.Nil(t, err)
}

func TestReadWriteDomainlessCredentialSpecErr(t *testing.T) {
	testCases := []struct {
		name                        string
		expectedReadError           error
		expectedWriteError          error
		expectedFunctionErrorString string
	}{
		{
			name:                        "mockReadErr",
			expectedReadError:           errors.New("mocked read error"),
			expectedWriteError:          errors.New("mocked write error"),
			expectedFunctionErrorString: "mocked read error",
		},
		{
			name:                        "mockReadErr2",
			expectedReadError:           errors.New("mocked read error"),
			expectedWriteError:          nil,
			expectedFunctionErrorString: "mocked read error",
		},
		{
			name:                        "mockWriteErr",
			expectedReadError:           nil,
			expectedWriteError:          errors.New("mocked write error"),
			expectedFunctionErrorString: "mocked write error",
		},
	}

	for _, tc := range testCases {
		readCredentialSpecImpl = func(filePath string) (map[string]interface{}, error) {
			return nil, tc.expectedReadError
		}

		writeCredentialSpecImpl = func(credSpec map[string]interface{}, outFilePath string, taskARN string) error {
			return tc.expectedWriteError
		}

		t.Run(tc.name, func(t *testing.T) {
			err := readWriteDomainlessCredentialSpec("random_file", "random_out_file", "random_taskArn")
			assert.EqualError(t, err, tc.expectedFunctionErrorString)
		})

		readCredentialSpecImpl = readCredentialSpec
		writeCredentialSpecImpl = writeCredentialSpec
	}
}

func TestReadCredentialSpecHappyCase(t *testing.T) {
	osReadFileImpl = func(string) ([]byte, error) {
		sampleCredSpecReturn := map[string]interface{}{"ActiveDirectoryConfig": "sample_value"}
		sampleCredSpecReturnBytes, _ := json.Marshal(sampleCredSpecReturn)
		return sampleCredSpecReturnBytes, nil
	}

	defer func() {
		osReadFileImpl = os.ReadFile
	}()

	credSpecUntyped, err := readCredentialSpec("random_file")
	assert.Nil(t, err)
	assert.NotNil(t, credSpecUntyped)
	credSpec := credSpecUntyped["ActiveDirectoryConfig"].(string)
	assert.Equal(t, "sample_value", credSpec)
}

func TestReadCredentialSpecUnmarshallErr(t *testing.T) {
	osReadFileImpl = func(string) ([]byte, error) {
		sampleCredSpecReturn := []int{1, 2, 3, 4}
		sampleCredSpecReturnBytes, _ := json.Marshal(sampleCredSpecReturn)
		return sampleCredSpecReturnBytes, nil
	}

	defer func() {
		osReadFileImpl = os.ReadFile
	}()

	credSpecUntyped, err := readCredentialSpec("random_file")
	assert.Nil(t, credSpecUntyped)
	assert.NotNil(t, err)
}

func TestReadCredentialSpecFileReadErr(t *testing.T) {
	testCases := []struct {
		name                string
		expectedErrorString string
	}{
		{
			name:                "readFileErr",
			expectedErrorString: "mocked error",
		},
	}

	for _, tc := range testCases {
		osReadFileImpl = func(string) ([]byte, error) {
			return nil, errors.New(tc.expectedErrorString)
		}

		t.Run(tc.name, func(t *testing.T) {
			credSpecUntyped, err := readCredentialSpec("random_file")
			assert.Nil(t, credSpecUntyped)
			assert.EqualError(t, err, tc.expectedErrorString)
		})

		osReadFileImpl = os.ReadFile
	}
}

func TestWriteCredentialSpecHappyCase(t *testing.T) {
	testCases := []struct {
		name               string
		taskArn            string
		outFilePath        string
		credSpec           map[string]interface{}
		credSpecAfterWrite map[string]interface{}
	}{
		{
			name:               "PluginInputCorrect",
			taskArn:            "123456",
			outFilePath:        "sample_path",
			credSpec:           map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{}"}}},
			credSpecAfterWrite: map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{\"regKeyPath\": \"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\AmazonECSCCGPlugin\\123456\"}"}}},
		},
		{
			name:               "PluginInputCorrect2",
			taskArn:            "123456",
			outFilePath:        "sample_path",
			credSpec:           map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{\"credentialArn\": \"arn:aws:ssm:us-west-2:632418168714:parameter/gmsa-plugin-input\"}"}}},
			credSpecAfterWrite: map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{\"credentialArn\": \"arn:aws:ssm:us-west-2:632418168714:parameter/gmsa-plugin-input\", \"regKeyPath\": \"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\AmazonECSCCGPlugin\\123456\",}"}}},
		},
		{
			name:               "PluginInputCorrectUnknownField",
			taskArn:            "123456",
			outFilePath:        "sample_path",
			credSpec:           map[string]interface{}{"Unknown_field": "Unknown_value", "ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{\"credentialArn\": \"arn:aws:ssm:us-west-2:632418168714:parameter/gmsa-plugin-input\"}"}}},
			credSpecAfterWrite: map[string]interface{}{"Unknown_field": "Unknown_value", "ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "{\"credentialArn\": \"arn:aws:ssm:us-west-2:632418168714:parameter/gmsa-plugin-input\", \"regKeyPath\": \"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\AmazonECSCCGPlugin\\123456\",}"}}},
		},
	}

	for _, tc := range testCases {
		osWriteFileImpl = func(outFilePath string, credSpec []byte, fileMode os.FileMode) error {
			if tc.outFilePath != outFilePath {
				return errors.New(fmt.Sprintf("Expected outfile path to be %s, intead got %s", tc.outFilePath, outFilePath))
			}

			if filePerm != fileMode {
				return errors.New(fmt.Sprintf("Expected fileMode to be %d, intead got %d", filePerm, fileMode))
			}

			if unMarshalledBytes, err := json.Marshal(tc.credSpecAfterWrite); err != nil || !bytes.Equal(unMarshalledBytes, credSpec) {
				return errors.New("credSpec does not contain correct contents after domainless gMSA write")
			}

			return nil
		}

		t.Run(tc.name, func(t *testing.T) {
			err := writeCredentialSpec(tc.credSpec, tc.outFilePath, tc.taskArn)
			assert.NotNil(t, err)
		})

		osWriteFileImpl = os.WriteFile
	}
}

func TestWriteCredentialSpecErr(t *testing.T) {
	osWriteFileImpl = func(string, []byte, os.FileMode) error {
		return nil
	}
	defer func() {
		osWriteFileImpl = os.WriteFile
	}()

	testCases := []struct {
		name                string
		credSpec            map[string]interface{}
		expectedErrorString string
	}{
		{
			name:                "invalid_ActiveDirectoryConfig",
			credSpec:            map[string]interface{}{"wrong_key": ""},
			expectedErrorString: "Unable to parse ActiveDirectoryConfig from credential spec",
		},
		{
			name:                "invalid_ActiveDirectoryConfigType",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": "wrong_value_type"},
			expectedErrorString: "Unable to marshal untyped object activeDirectoryConfigUntyped to type map[string]interface{}",
		},
		{
			name:                "invalid_HostAccountConfig",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"wrong_key": ""}},
			expectedErrorString: "Unable to parse HostAccountConfig from credential spec",
		},
		{
			name:                "invalid_HostAccountConfigType",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": "wrong_value_type"}},
			expectedErrorString: "Unable to marshal untyped object hostAccountConfigUntyped to type map[string]interface{}",
		},
		{
			name:                "invalid_PluginInput",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"wrong_key": ""}}},
			expectedErrorString: "Unable to parse PluginInput from credential spec",
		},
		{
			name:                "invalid_PluginInputType",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": 0}}},
			expectedErrorString: "Unable to marshal untyped object pluginInputStringUntyped to type string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeCredentialSpec(tc.credSpec, "", "")
			assert.EqualError(t, err, tc.expectedErrorString)
		})
	}
}

func TestWriteCredentialSpecJSONMarshallErr(t *testing.T) {
	osWriteFileImpl = func(string, []byte, os.FileMode) error {
		return nil
	}
	defer func() {
		osWriteFileImpl = os.WriteFile
	}()

	testCases := []struct {
		name                string
		credSpec            map[string]interface{}
		expectedErrorString string
	}{
		{
			name:                "invalid_PluginInputParsed",
			credSpec:            map[string]interface{}{"ActiveDirectoryConfig": map[string]interface{}{"HostAccountConfig": map[string]interface{}{"PluginInput": "wrong_plugininput_value"}}},
			expectedErrorString: "Unable to marshal untyped object pluginInputStringUntyped to type string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeCredentialSpec(tc.credSpec, "", "")
			assert.NotNil(t, err)
		})
	}
}
