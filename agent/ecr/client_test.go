// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// ecr_test packge to avoid test dependency cycle on ecr/mocks
package ecr_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/async/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	"github.com/aws/amazon-ecs-agent/agent/ecr/mocks"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// test constants
const (
	testRegistryId    = "testRegistryId"
	testProxyEndpoint = "testProxyEndpoint"
	testToken         = "testToken"
)

// test suite struct for handling mocks and test client
type GetAuthorizationTokenTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *mock_ecr.MockECRSDK
	mockCache  *mock_async.MockCache
	ecrClient  ecr.ECRClient
}

// test suite setup & teardown
func TestGetAuthorizationTokenSuite(t *testing.T) {
	suite.Run(t, new(GetAuthorizationTokenTestSuite))
}

func (suite *GetAuthorizationTokenTestSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockClient = mock_ecr.NewMockECRSDK(suite.ctrl)
	suite.mockCache = mock_async.NewMockCache(suite.ctrl)
	suite.ecrClient = ecr.NewECRClient(suite.mockClient, suite.mockCache)
}

func (suite *GetAuthorizationTokenTestSuite) TeardownTest() {
	suite.ctrl.Finish()
}

// begin tests
func (suite *GetAuthorizationTokenTestSuite) TestGetAuthorizationTokenCacheHit() {
	testAuthData := &ecrapi.AuthorizationData{
		ProxyEndpoint:      aws.String(testProxyEndpoint),
		AuthorizationToken: aws.String(testToken),
		ExpiresAt:          aws.Time(time.Now().Add(12 * time.Hour)),
	}

	suite.mockCache.EXPECT().Get(testRegistryId).Return(testAuthData, true)

	authorizationData, err := suite.ecrClient.GetAuthorizationToken(testRegistryId)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testAuthData, authorizationData)
}

func (suite *GetAuthorizationTokenTestSuite) TestGetAuthorizationTokenCacheMiss() {
	testAuthData := &ecrapi.AuthorizationData{
		ProxyEndpoint:      aws.String(testProxyEndpoint),
		AuthorizationToken: aws.String(testToken),
		ExpiresAt:          aws.Time(time.Now().Add(12 * time.Hour)),
	}

	suite.mockClient.EXPECT().GetAuthorizationToken(
		&ecrapi.GetAuthorizationTokenInput{
			RegistryIds: []*string{aws.String(testRegistryId)},
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{testAuthData},
	}, nil)

	suite.mockCache.EXPECT().Get(testRegistryId).Return(nil, false)
	suite.mockCache.EXPECT().Set(testRegistryId, testAuthData)

	authorizationData, err := suite.ecrClient.GetAuthorizationToken(testRegistryId)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testAuthData, authorizationData)
}

func (suite *GetAuthorizationTokenTestSuite) TestGetAuthorizationTokenCacheHitExpiredToken() {
	expiredAuthData := &ecrapi.AuthorizationData{
		ProxyEndpoint:      aws.String(testProxyEndpoint),
		AuthorizationToken: aws.String(testToken),
		ExpiresAt:          aws.Time(time.Now().Add(-5 * time.Minute)),
	}

	testAuthData := &ecrapi.AuthorizationData{
		ProxyEndpoint:      aws.String(testProxyEndpoint),
		AuthorizationToken: aws.String(testToken),
		ExpiresAt:          aws.Time(time.Now().Add(12 * time.Hour)),
	}

	suite.mockClient.EXPECT().GetAuthorizationToken(
		&ecrapi.GetAuthorizationTokenInput{
			RegistryIds: []*string{aws.String(testRegistryId)},
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{testAuthData},
	}, nil)

	suite.mockCache.EXPECT().Get(testRegistryId).Return(expiredAuthData, true)
	suite.mockCache.EXPECT().Set(testRegistryId, testAuthData)

	authorizationData, err := suite.ecrClient.GetAuthorizationToken(testRegistryId)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testAuthData, authorizationData)
}

func (suite *GetAuthorizationTokenTestSuite) TestGetAuthorizationTokenMissingAuthData() {
	suite.mockClient.EXPECT().GetAuthorizationToken(
		&ecrapi.GetAuthorizationTokenInput{
			RegistryIds: []*string{aws.String(testRegistryId)},
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{},
	}, nil)

	suite.mockCache.EXPECT().Get(testRegistryId).Return(nil, false)

	authorizationData, err := suite.ecrClient.GetAuthorizationToken(testRegistryId)
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), authorizationData)
}

func (suite *GetAuthorizationTokenTestSuite) TestGetAuthorizationTokenError() {
	suite.mockClient.EXPECT().GetAuthorizationToken(
		&ecrapi.GetAuthorizationTokenInput{
			RegistryIds: []*string{aws.String(testRegistryId)},
		}).Return(nil, errors.New("Nope Nope Nope"))

	suite.mockCache.EXPECT().Get(testRegistryId).Return(nil, false)

	authorizationData, err := suite.ecrClient.GetAuthorizationToken(testRegistryId)
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), authorizationData)
}

func (suite *GetAuthorizationTokenTestSuite) TestIsTokenValid() {

	var testAuthTimes = []struct {
		expireIn time.Duration
		expected bool
	}{
		{-1 * time.Minute, false},
		{time.Duration(0), false},
		{1 * time.Minute, false},
		{ecr.MinimumJitterDuration, false},
		{ecr.MaximumJitterDuration + (1 * time.Second), true},
	}

	for _, testCase := range testAuthTimes {
		testAuthData := &ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String(testProxyEndpoint),
			AuthorizationToken: aws.String(testToken),
			ExpiresAt:          aws.Time(time.Now().Add(testCase.expireIn)),
		}

		actual := suite.ecrClient.IsTokenValid(testAuthData)

		assert.Equal(suite.T(), testCase.expected, actual,
			fmt.Sprintf("Expected IsTokenValid to be %t, got %t: for expiraing at %s", testCase.expected, actual, testCase.expireIn))
	}
}
