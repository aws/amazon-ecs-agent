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
	"sync"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mockasmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mockasmiface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mockcredentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mockfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mockssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"

	"testing"

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
		"ssmARN": "/var/credentials-fetcher/krbdir/123456/ccname_webapp01_xyz",
		"asmARN": "/var/credentials-fetcher/krbdir/123456/ccname_webapp02_xyz",
	}

	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
		"asmARN": {serviceAccountName: "webapp02", domainName: "contoso.com"},
	}

	credspecRes := &CredentialSpecResource{
		CredSpecMap:           credSpecMapData,
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
	asmClientCreator := mockasmfactory.NewMockClientCreator(ctrl)
	credspecRes := &CredentialSpecResource{
		knownStatusUnsafe:   resourcestatus.ResourceCreated,
		desiredStatusUnsafe: resourcestatus.ResourceCreated,
	}
	credspecRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
			ASMClientCreator:   asmClientCreator,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	assert.NotNil(t, credspecRes.credentialsManager)
	assert.NotNil(t, credspecRes.ssmClientCreator)
	assert.NotNil(t, credspecRes.asmClientCreator)
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
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		ServiceAccountInfoMap:      map[string]ServiceAccountInfo{},
		taskARN:                    taskARN,
		credentialSpecContainerMap: credentialSpecContainerMap,
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
	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/ccname_webapp01_xyz"

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

func TestHandleSSMCredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecSSMARN := "arn:aws:ssm:parameter/test"
	ssmCredentialSpec := "credentialspec:arn:aws:ssm:us-west-2:123456789012:parameter/test"

	cs := &CredentialSpecResource{
		terminalReason: "failed",
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

func TestHandleASMCredentialSpecFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	asmClientCreater := mockasmfactory.NewMockClientCreator(ctrl)
	mockASMClient := mockasmiface.NewMockSecretsManagerAPI(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	containerName := "webapp"

	credentialSpecASMARN := "arn:aws:secretsmanager:us-west-2:123456789012:secret:test-8mJ3EJ"
	asmCredentialSpec := "credentialspec:arn:aws:secretsmanager:us-west-2:123456789012:secret:test-8mJ3EJ"

	credentialSpecContainerMap := map[string]string{
		credentialSpecASMARN: containerName,
	}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		ServiceAccountInfoMap:      map[string]ServiceAccountInfo{},
		taskARN:                    taskARN,
		credentialSpecContainerMap: credentialSpecContainerMap,
	}

	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreater,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	testData := "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"
	asmClientOutput := &secretsmanager.GetSecretValueOutput{
		ARN:          aws.String("arn:aws:secretsmanager:us-west-2:618112483929:secret:test-8mJ3EJ"),
		Name:         aws.String("test"),
		VersionId:    aws.String("f3b693da-0204-4a02-8b94-ddfbe964ebcb"),
		SecretString: aws.String(testData),
	}
	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/ccname_webapp01_xyz"

	gomock.InOrder(
		asmClientCreater.EXPECT().NewASMClient(gomock.Any(), gomock.Any()).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Return(asmClientOutput, nil).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleASMCredentialspecFile(asmCredentialSpec, credentialSpecASMARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.NoError(t, err)

	cs.CredSpecMap[credentialSpecASMARN] = expectedKerberosTicketPath
	actualKerberosTicketPath, err := cs.GetTargetMapping(credentialSpecASMARN)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}

func TestHandleASMCredentialspecFileARNParseErr(t *testing.T) {
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}
	credentialSpecASMARN := "arn:aws:/test"
	asmCredentialSpec := "credentialspec:arn:aws:/test"

	var termReason string
	cs := &CredentialSpecResource{
		terminalReason: termReason,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, 1)
	go cs.handleASMCredentialspecFile(asmCredentialSpec, credentialSpecASMARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestHandleASMCredentialSpecFileGetASMSecretValueErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mockcredentials.NewMockManager(ctrl)
	asmClientCreater := mockasmfactory.NewMockClientCreator(ctrl)
	mockASMClient := mockasmiface.NewMockSecretsManagerAPI(ctrl)
	iamCredentials := credentials.IAMRoleCredentials{
		CredentialsID: "test-cred-id",
	}

	credentialSpecASMARN := "arn:aws:secretsmanager:us-west-2:123456789012:secret:test-8mJ3EJ"
	asmCredentialSpec := "credentialspec:arn:aws:secretsmanager:us-west-2:123456789012:secret:test-8mJ3EJ"

	credentialSpecContainerMap := map[string]string{credentialSpecASMARN: "webapp"}

	cs := &CredentialSpecResource{
		knownStatusUnsafe:          resourcestatus.ResourceCreated,
		desiredStatusUnsafe:        resourcestatus.ResourceCreated,
		CredSpecMap:                map[string]string{},
		taskARN:                    taskARN,
		credentialSpecContainerMap: credentialSpecContainerMap,
	}
	cs.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreater,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)

	gomock.InOrder(
		asmClientCreater.EXPECT().NewASMClient(gomock.Any(), gomock.Any()).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Return(nil, errors.New("test-error")).Times(1),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	errorEvents := make(chan error, len(cs.credentialSpecContainerMap))
	go cs.handleASMCredentialspecFile(asmCredentialSpec, credentialSpecASMARN, iamCredentials, &wg, errorEvents)
	wg.Wait()
	close(errorEvents)

	err := <-errorEvents
	assert.Error(t, err)
}

func TestGetName(t *testing.T) {
	cs := &CredentialSpecResource{}

	assert.Equal(t, ResourceName, cs.GetName())
}

func TestGetTargetMapping(t *testing.T) {
	inputCredSpec := "credentialspec:ssmARN"
	outputKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/ccname_webapp01_xyz"

	credSpecMapData := map[string]string{
		"credentialspec:ssmARN": "/var/credentials-fetcher/krbdir/123456/ccname_webapp01_xyz",
	}
	credentialsFetcherInfoMap := map[string]ServiceAccountInfo{
		"credentialspec:ssmARN": {serviceAccountName: "webapp01", domainName: "contoso.com"},
	}

	cs := &CredentialSpecResource{
		CredSpecMap:           credSpecMapData,
		ServiceAccountInfoMap: credentialsFetcherInfoMap,
	}

	targetKerberosTicketPath, err := cs.GetTargetMapping(inputCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, outputKerberosTicketPath, targetKerberosTicketPath)
}

func TestGetTargetMappingErr(t *testing.T) {
	cs := &CredentialSpecResource{
		CredSpecMap: map[string]string{},
	}

	targetKerberosTicketPath, err := cs.GetTargetMapping("testcredspec")
	assert.Error(t, err)
	assert.Empty(t, targetKerberosTicketPath)
}
