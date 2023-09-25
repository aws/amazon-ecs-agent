//go:build linux
// +build linux

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
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"

	mock_asm_factory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks"
	mockfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mockssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mockcredentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/aws-sdk-go/aws"
	s3sdk "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN = "arn:aws:ecs:us-west-2:123456789012:task/12345-678901234-56789"
)

func TestClearCredentialSpecDataHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credSpecMapData := map[string]string{
		"ssmARN": "/var/credentials-fetcher/krbdir/123456/webapp01",
		"asmARN": "/var/credentials-fetcher/krbdir/123456/webapp02",
	}

	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
		"asmARN": {serviceAccountName: "webapp02", domainName: "contoso.com"},
	}

	credspecRes := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
	}

	err := credspecRes.Cleanup()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(credspecRes.CredSpecMap))
	assert.Equal(t, 0, len(credspecRes.ServiceAccountInfoMap))
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	ssmClientCreator := mockfactory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	asmClientCreator := mock_asm_factory.NewMockClientCreator(ctrl)
	credspecRes := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:   resourcestatus.ResourceCreated,
			desiredStatusUnsafe: resourcestatus.ResourceCreated,
		},
	}
	credspecRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			S3ClientCreator:    s3ClientCreator,
			ASMClientCreator:   asmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	assert.NotNil(t, credspecRes.credentialsManager)
	assert.NotNil(t, credspecRes.ssmClientCreator)
	assert.NotNil(t, credspecRes.s3ClientCreator)
	assert.NotNil(t, credspecRes.secretsmanagerClientCreator)
	assert.NotNil(t, credspecRes.resourceStatusToTransitionFunction)
}

func TestHandleSSMCredentialspecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	ssmClientCreator := mockfactory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mockssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{
		credentialSpecSSMARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}

	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			{
				Name:  aws.String("/test"),
				Value: aws.String(testData),
			},
		},
	}
	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
	)

	var wg sync.WaitGroup
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	wg.Add(1)
	go cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials, &wg, errorEvents)

	wg.Wait()
	close(errorEvents)
	err := <-errorEvents
	assert.NoError(t, err)

	cs.CredSpecMap[credentialSpecSSMARN] = expectedKerberosTicketPath

	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecSSMARN)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestHandleSSMDomainlessCredentialspecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	ssmClientCreator := mockfactory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mockssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspecdomainless:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{
		credentialSpecSSMARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		isDomainlessGmsa:      true,
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}

	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "{ \"CmsPlugins\": [ \"ActiveDirectory\" ], \"DomainJoinConfig\": { \"Sid\": \"S-1-5-21-4066351383-705263209-1606769140\", \"MachineAccountName\": \"WebApp01\", \"Guid\": \"ac822f13-583e-49f7-aa7b-284f9a8c97b6\", \"DnsTreeName\": \"contoso.com\", \"DnsName\": \"contoso.com\", \"NetBiosName\": \"contoso\" }, \"ActiveDirectoryConfig\": { \"GroupManagedServiceAccounts\": [ { \"Name\": \"WebApp01\", \"Scope\": \"contoso.com\" }, { \"Name\": \"WebApp01\", \"Scope\": \"contoso\" } ], \"HostAccountConfig\": { \"PortableCcgVersion\": \"1\", \"PluginGUID\": \"{859E1386-BDB4-49E8-85C7-3070B13920E1}\", \"PluginInput\": {\"CredentialArn\": \"arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO\"} } }}"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			{
				Name:  aws.String("/test"),
				Value: aws.String(testData),
			},
		},
	}
	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
	)

	var wg sync.WaitGroup
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	wg.Add(1)
	go cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials, &wg, errorEvents)

	wg.Wait()
	close(errorEvents)
	err := <-errorEvents
	assert.NoError(t, err)

	cs.CredSpecMap[credentialSpecSSMARN] = expectedKerberosTicketPath

	expectedOutput := ServiceAccountInfo{
		serviceAccountName:    "WebApp01",
		domainName:            "contoso.com",
		domainlessGmsaUserArn: "arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO",
		credentialSpecContent: testData,
	}

	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecSSMARN)
	assert.Equal(t, cs.ServiceAccountInfoMap[ssmCredentialSpec], expectedOutput)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestHandleSSMCredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			terminalReason: "failed",
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, 1)
	go cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials, &wg, errorEvents)

	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestHandleSSMCredentialspecFileGetSSMParamErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	ssmClientCreator := mockfactory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mockssmiface.NewMockSSMClient(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{credentialSpecSSMARN: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(nil, errors.New("test-error")).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleSSMCredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials, &wg, errorEvents)

	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestHandleS3CredentialSpecFileGetS3SecretValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecS3ARN := "arn:aws:s3:::gmsacredspec/contoso_webapp01.json"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::gmsacredspec/contoso_webapp01.json"

	credentialSpecContainerMap := map[string]string{
		credentialSpecS3ARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"
	testData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"

	s3GetObjectResponse := &s3sdk.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(testData)),
	}
	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3Client(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockS3Client.EXPECT().GetObject(gomock.Any()).Return(s3GetObjectResponse, nil).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.NoError(t, err)

	cs.CredSpecMap[credentialSpecS3ARN] = expectedKerberosTicketPath
	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecS3ARN)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestHandleS3DomainlessCredentialSpecFileGetS3SecretValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecS3ARN := "arn:aws:s3:::gmsacredspec/contoso_webapp01.json"
	s3CredentialSpec := "credentialspecdomainless:arn:aws:s3:::gmsacredspec/contoso_webapp01.json"

	credentialSpecContainerMap := map[string]string{
		credentialSpecS3ARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
		isDomainlessGmsa:      true,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"
	testData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4066351383-705263209-1606769140\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"ac822f13-583e-49f7-aa7b-284f9a8c97b6\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}],\"HostAccountConfig\":{\"PortableCcgVersion\":\"1\",\"PluginGUID\":\"{859E1386-BDB4-49E8-85C7-3070B13920E1}\",\"PluginInput\":{\"CredentialArn\":\"arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO\"}}}}"

	s3GetObjectResponse := &s3sdk.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(testData)),
	}
	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3Client(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockS3Client.EXPECT().GetObject(gomock.Any()).Return(s3GetObjectResponse, nil).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.NoError(t, err)

	expectedOutput := ServiceAccountInfo{
		serviceAccountName:    "WebApp01",
		domainName:            "contoso.com",
		domainlessGmsaUserArn: "arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO",
		credentialSpecContent: testData,
	}

	cs.CredSpecMap[credentialSpecS3ARN] = expectedKerberosTicketPath
	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecS3ARN)
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, cs.ServiceAccountInfoMap[s3CredentialSpec])
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestHandleS3CredentialSpecFileGetS3SecretValueErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)
	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	credentialSpecS3ARN := "arn:aws:s3:::gmsacredspec/contoso_webapp01.json"
	s3CredentialSpec := "credentialspec:arn:aws:s3:::gmsacredspec/contoso_webapp01.json"

	credentialSpecContainerMap := map[string]string{credentialSpecS3ARN: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
			S3ClientCreator:    s3ClientCreator,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		s3ClientCreator.EXPECT().NewS3Client(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockS3Client, nil),
		mockS3Client.EXPECT().GetObject(gomock.Any()).Return(nil, errors.New("test-error")).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleS3CredentialspecFile(s3CredentialSpec, credentialSpecS3ARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestHandleS3CredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:s3:::contoso_webapp01.json"
	ssmCredentialSpec := "credentialspec:arn:aws:s3:::contoso_webapp01.json"

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			terminalReason: "failed",
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, 1)
	go cs.handleS3CredentialspecFile(ssmCredentialSpec, credentialSpecSSMARN, iamCredentials, &wg, errorEvents)

	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestHandleCredentialSpecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCredSpecFilePath := "/tmp/webapp01.json"
	credentialsManager := mockcredentials.NewMockManager(ctrl)

	credentialSpecARN := "file:///tmp/webapp01.json"
	CredentialSpec := "credentialspec:file:///tmp/webapp01.json"

	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "WebApp01",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "contoso.com",
                             "DnsName":  "contoso.com",
                             "NetBiosName":  "contoso"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "WebApp01",
                                                                          "Scope":  "contoso.com"
                                                                      }
                                                                  ]
                              }
}`)

	writeErr := ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	assert.NoError(t, writeErr)

	credentialSpecContainerMap := map[string]string{credentialSpecARN: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleCredentialspecFile(CredentialSpec, credentialSpecARN, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.NoError(t, err)

	// Cleanup the test file
	err = os.RemoveAll(testCredSpecFilePath)
	assert.NoError(t, err)
}

func TestHandleDomainlessCredentialSpecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCredSpecFilePath := "/tmp/webapp01.json"
	credentialsManager := mockcredentials.NewMockManager(ctrl)

	credentialSpecARN := "file:///tmp/webapp01.json"
	CredentialSpec := "credentialspecdomainless:file:///tmp/webapp01.json"

	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "WebApp01",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "contoso.com",
                             "DnsName":  "contoso.com",
                             "NetBiosName":  "contoso"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "WebApp01",
                                                                          "Scope":  "contoso.com"
                                                                      }
                                                                  ],
								"HostAccountConfig": {
        							"PortableCcgVersion": "1",
        							"PluginGUID": "{859E1386-BDB4-49E8-85C7-3070B13920E1}",
       								 "PluginInput": {
          									"CredentialArn": "arn:aws:secretsmanager:us-west-2:618112483929:secret:gMSAUserSecret-PwmPaO"
        							}
     							 }
                              }
}`)

	writeErr := ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	assert.NoError(t, writeErr)

	credentialSpecContainerMap := map[string]string{credentialSpecARN: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
		isDomainlessGmsa:      true,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleCredentialspecFile(CredentialSpec, credentialSpecARN, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.NoError(t, err)

	expectedOutput := ServiceAccountInfo{
		serviceAccountName:    "WebApp01",
		domainName:            "contoso.com",
		domainlessGmsaUserArn: "arn:aws:secretsmanager:us-west-2:618112483929:secret:gMSAUserSecret-PwmPaO",
	}

	assert.Equal(t, cs.ServiceAccountInfoMap[CredentialSpec].serviceAccountName, expectedOutput.serviceAccountName)
	assert.Equal(t, cs.ServiceAccountInfoMap[CredentialSpec].domainlessGmsaUserArn, expectedOutput.domainlessGmsaUserArn)

	// Cleanup the test file
	err = os.RemoveAll(testCredSpecFilePath)
	assert.NoError(t, err)
}

func TestHandleCredentialSpecFileErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)

	credentialSpecARN := "/tmp/webapp01.json"
	CredentialSpec := "credentialspec:/tmp/webapp01.json"

	credentialSpecContainerMap := map[string]string{credentialSpecARN: "webapp"}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleCredentialspecFile(CredentialSpec, credentialSpecARN, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestGetName(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	assert.Equal(t, ResourceName, cs.GetName())
}

func TestGetTargetMapping(t *testing.T) {
	inputCredSpec := "credentialspec:ssmARN"
	outputKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"

	credSpecMapData := map[string]string{
		"credentialspec:ssmARN": "/var/credentials-fetcher/krbdir/123456/webapp01",
	}
	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"credentialspec:ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
	}

	targetKerberosTicketPath, err := cs.GetTargetMapping(inputCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, outputKerberosTicketPath, targetKerberosTicketPath)
}

func TestGetTargetMappingErr(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: map[string]string{},
		},
	}

	targetKerberosTicketPath, err := cs.GetTargetMapping("testcredspec")
	assert.Error(t, err)
	assert.Empty(t, targetKerberosTicketPath)
}

func TestUpdateTargetMapping(t *testing.T) {
	inputCredSpec := "credentialspec:ssmARN"
	credSpecData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"

	credSpecMapData := map[string]string{
		"credentialspec:ssmARN": "/var/credentials-fetcher/krbdir/123456/webapp01",
	}
	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"credentialspec:ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
		isDomainlessGmsa:      false,
	}

	cs.updateCredSpecMapping(inputCredSpec, credSpecData)

	expectedOutput := ServiceAccountInfo{
		serviceAccountName:    "WebApp01",
		domainName:            "contoso.com",
		credentialSpecContent: credSpecData,
	}

	assert.Equal(t, cs.ServiceAccountInfoMap[inputCredSpec], expectedOutput)
}

func TestUpdateTargetMappingDomainless(t *testing.T) {
	inputCredSpec := "credentialspec:ssmARN"
	credSpecData := "{ \"CmsPlugins\": [ \"ActiveDirectory\" ], \"DomainJoinConfig\": { \"Sid\": \"S-1-5-21-4066351383-705263209-1606769140\", \"MachineAccountName\": \"WebApp01\", \"Guid\": \"ac822f13-583e-49f7-aa7b-284f9a8c97b6\", \"DnsTreeName\": \"contoso.com\", \"DnsName\": \"contoso.com\", \"NetBiosName\": \"contoso\" }, \"ActiveDirectoryConfig\": { \"GroupManagedServiceAccounts\": [ { \"Name\": \"WebApp01\", \"Scope\": \"contoso.com\" }, { \"Name\": \"WebApp01\", \"Scope\": \"contoso\" } ], \"HostAccountConfig\": { \"PortableCcgVersion\": \"1\", \"PluginGUID\": \"{859E1386-BDB4-49E8-85C7-3070B13920E1}\", \"PluginInput\": {\"CredentialArn\": \"arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO\"} } }}"

	credSpecMapData := map[string]string{
		"credentialspec:ssmARN": "/var/credentials-fetcher/krbdir/123456/webapp01",
	}
	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"credentialspec:ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
		isDomainlessGmsa:      true,
	}

	cs.updateCredSpecMapping(inputCredSpec, credSpecData)

	expectedOutput := ServiceAccountInfo{
		serviceAccountName:    "WebApp01",
		domainName:            "contoso.com",
		domainlessGmsaUserArn: "arn:aws:secretsmanager:us-west-2:123456789:secret:gMSAUserSecret-PwmPaO",
		credentialSpecContent: credSpecData,
	}

	assert.Equal(t, cs.ServiceAccountInfoMap[inputCredSpec], expectedOutput)
}

func TestUpdateTargetMappingError(t *testing.T) {
	inputCredSpec := "credentialspec:ssmARN"
	credSpecData := "{\"xyz\"}"

	credSpecMapData := map[string]string{
		"credentialspec:ssmARN": "/var/credentials-fetcher/krbdir/123456/webapp01",
	}
	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"credentialspec:ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			CredSpecMap: credSpecMapData,
		},
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
		isDomainlessGmsa:      true,
	}

	err := cs.updateCredSpecMapping(inputCredSpec, credSpecData)

	assert.EqualError(t, err, "error unmarshalling credentialspec domainless {\"xyz\"} : invalid character '}' after object key")
}

func TestSkipCredentialFetcherInvocation(t *testing.T) {
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	ssmClientCreator := mockfactory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mockssmiface.NewMockSSMClient(ctrl)
	taskRoleCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			CredentialsID: "test-cred-id",
		},
	}

	containerName := "webapp"

	credentialSpecSSMARN := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{
		credentialSpecSSMARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ServiceAccountInfoMap: map[string]ServiceAccountInfo{},
	}

	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"
	ssmClientOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			{
				Name:  aws.String("/test"),
				Value: aws.String(testData),
			},
		},
	}
	expectedKerberosTicketPath := "/tmp/tgt"

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(taskRoleCredentials, true).Times(1),
		ssmClientCreator.EXPECT().NewSSMClient(gomock.Any(), gomock.Any()).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Return(ssmClientOutput, nil).Times(1),
	)

	err := cs.Create()

	assert.NoError(t, err)

	cs.CredSpecMap[credentialSpecSSMARN] = expectedKerberosTicketPath

	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecSSMARN)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestUpdateRegionForTask(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			taskARN: "arn:aws:ecs:us-west-2:123456789:task-definition/contoso-demo:1",
			region:  "",
		},
	}

	err := cs.UpdateRegionFromTask()
	assert.NoError(t, err)
	assert.Equal(t, cs.region, "us-west-2")
}

func TestUpdateRegionForTaskMaliformedInput(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			taskARN: "arn:aws:ecs/123456789:task-definition/contoso-demo:1",
			region:  "",
		},
	}

	err := cs.UpdateRegionFromTask()
	assert.Error(t, err)
}
