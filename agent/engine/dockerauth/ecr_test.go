// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dockerauth

import (
	"encoding/base64"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/ecr/mocks"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
)

func TestNewAuthProviderECRAuthNoAuth(t *testing.T) {
	provider := NewECRAuthProvider(nil, nil)
	ecrProvider, ok := provider.(*ecrAuthProvider)
	if !ok {
		t.Error("Should have returned ecrAuthProvider")
	}
	if ecrProvider.authData != nil {
		t.Error("authData should be nil")
	}
}

func TestNewAuthProviderECRAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	factory := mock_ecr.NewMockECRFactory(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}

	factory.EXPECT().GetClient(authData.Region, authData.EndpointOverride)

	provider := NewECRAuthProvider(authData, factory)
	_, ok := provider.(*ecrAuthProvider)
	if !ok {
		t.Error("Should have returned ecrAuthProvider")
	}
}

func TestGetAuthConfigSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}
	proxyEndpoint := "proxy"
	username := "username"
	password := "password"

	provider := ecrAuthProvider{
		client:   client,
		authData: authData,
	}

	client.EXPECT().GetAuthorizationToken(gomock.Any()).Do(
		func(input *ecrapi.GetAuthorizationTokenInput) {
			if input == nil {
				t.Fatal("Called with nil input")
			}
			if len(input.RegistryIds) != 1 {
				t.Fatalf("Unexpected number of RegistryIds, expected 1 but got %d", len(input.RegistryIds))
			}
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{
			&ecrapi.AuthorizationData{
				ProxyEndpoint:      aws.String(proxyEndpointScheme + proxyEndpoint),
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			},
		},
	}, nil)

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	if reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatal("Authconfig unexpectedly empty")
	}
	if authconfig.Username != username {
		t.Errorf("Expected username to be %s, but was %s", username, authconfig.Username)
	}
	if authconfig.Password != password {
		t.Errorf("Expected password to be %s, but was %s", password, authconfig.Password)
	}
}

func TestGetAuthConfigNoMatchAuthorizationToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}
	proxyEndpoint := "proxy"
	username := "username"
	password := "password"

	provider := ecrAuthProvider{
		client:   client,
		authData: authData,
	}

	client.EXPECT().GetAuthorizationToken(gomock.Any()).Do(
		func(input *ecrapi.GetAuthorizationTokenInput) {
			if input == nil {
				t.Fatal("Called with nil input")
			}
			if len(input.RegistryIds) != 1 {
				t.Fatalf("Unexpected number of RegistryIds, expected 1 but got %d", len(input.RegistryIds))
			}
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{
			&ecrapi.AuthorizationData{
				ProxyEndpoint:      aws.String(proxyEndpointScheme + "notproxy"),
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			},
		},
	}, nil)

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err == nil {
		t.Fatal("Expected error to be present, but was nil", err)
	}
	t.Log(err)
	if !reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatalf("Expected Authconfig to be empty, but was %v", authconfig)
	}
}

func TestGetAuthConfigBadBase64(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}
	proxyEndpoint := "proxy"
	username := "username"
	password := "password"

	provider := ecrAuthProvider{
		client:   client,
		authData: authData,
	}

	client.EXPECT().GetAuthorizationToken(gomock.Any()).Do(
		func(input *ecrapi.GetAuthorizationTokenInput) {
			if input == nil {
				t.Fatal("Called with nil input")
			}
			if len(input.RegistryIds) != 1 {
				t.Fatalf("Unexpected number of RegistryIds, expected 1 but got %d", len(input.RegistryIds))
			}
		}).Return(&ecrapi.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecrapi.AuthorizationData{
			&ecrapi.AuthorizationData{
				ProxyEndpoint:      aws.String(proxyEndpoint),
				AuthorizationToken: aws.String(username + ":" + password),
			},
		},
	}, nil)

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err == nil {
		t.Fatal("Expected error to be present, but was nil", err)
	}
	t.Log(err)
	if !reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatalf("Expected Authconfig to be empty, but was %v", authconfig)
	}
}

func TestGetAuthConfigMissingResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}
	proxyEndpoint := "proxy"

	provider := ecrAuthProvider{
		client:   client,
		authData: authData,
	}

	client.EXPECT().GetAuthorizationToken(gomock.Any()).Do(
		func(input *ecrapi.GetAuthorizationTokenInput) {
			if input == nil {
				t.Fatal("Called with nil input")
			}
			if len(input.RegistryIds) != 1 {
				t.Fatalf("Unexpected number of RegistryIds, expected 1 but got %d", len(input.RegistryIds))
			}
		})

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err == nil {
		t.Fatal("Expected error to be present, but was nil", err)
	}
	t.Log(err)
	if !reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatalf("Expected Authconfig to be empty, but was %v", authconfig)
	}
}

func TestGetAuthConfigECRError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	authData := &api.ECRAuthData{
		Region:           "us-west-2",
		RegistryId:       "0123456789012",
		EndpointOverride: "my.endpoint",
	}
	proxyEndpoint := "proxy"

	provider := ecrAuthProvider{
		client:   client,
		authData: authData,
	}

	client.EXPECT().GetAuthorizationToken(gomock.Any()).Do(
		func(input *ecrapi.GetAuthorizationTokenInput) {
			if input == nil {
				t.Fatal("Called with nil input")
			}
			if len(input.RegistryIds) != 1 {
				t.Fatalf("Unexpected number of RegistryIds, expected 1 but got %d", len(input.RegistryIds))
			}
		}).Return(nil, errors.New("test error"))

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err == nil {
		t.Fatal("Expected error to be present, but was nil", err)
	}
	t.Log(err)
	if !reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatalf("Expected Authconfig to be empty, but was %v", authconfig)
	}
}

func TestGetAuthConfigNoAuthData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_ecr.NewMockECRSDK(ctrl)

	proxyEndpoint := "proxy"

	provider := ecrAuthProvider{
		client:   client,
		authData: nil,
	}

	authconfig, err := provider.GetAuthconfig(proxyEndpoint + "/myimage")
	if err == nil {
		t.Fatal("Expected error to be present, but was nil", err)
	}
	t.Log(err)
	if !reflect.DeepEqual(authconfig, docker.AuthConfiguration{}) {
		t.Fatalf("Expected Authconfig to be empty, but was %v", authconfig)
	}
}
