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

// Package ecr helps generate clients to talk to the ECR API
package ecr

import (
	"net/http"
	"sync"
	"time"

	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

// ECRSDK is an interface that specifies the subset of the AWS Go SDK's ECR
// client that the Agent uses.  This interface is meant to allow injecting a
// mock for testing.
type ECRSDK interface {
	GetAuthorizationToken(*ecrapi.GetAuthorizationTokenInput) (*ecrapi.GetAuthorizationTokenOutput, error)
}

type ECRFactory interface {
	GetClient(region, endpointOverride string) ECRSDK
}

type ecrFactory struct {
	httpClient *http.Client

	clientsLock sync.Mutex
	clients     map[cacheKey]ECRSDK
}

type cacheKey struct {
	region           string
	endpointOverride string
}

const (
	roundtripTimeout = 5 * time.Second
)

// NewECRFactory returns an ECRFactory capable of producing ECRSDK clients
func NewECRFactory(acceptInsecureCert bool) ECRFactory {
	return &ecrFactory{
		httpClient: httpclient.New(roundtripTimeout, acceptInsecureCert),
		clients:    make(map[cacheKey]ECRSDK),
	}
}

// GetClient returns the correct region- and endpoint-aware client
func (factory *ecrFactory) GetClient(region, endpointOverride string) ECRSDK {
	key := cacheKey{region: region, endpointOverride: endpointOverride}
	client, ok := factory.clients[key]
	if ok {
		return client
	}

	factory.clientsLock.Lock()
	defer factory.clientsLock.Unlock()
	client, ok = factory.clients[key]
	if ok {
		return client
	}
	client = factory.newClient(region, endpointOverride)
	factory.clients[key] = client
	return client
}

func (factory *ecrFactory) newClient(region, endpointOverride string) ECRSDK {
	var ecrConfig aws.Config
	ecrConfig.Region = &region
	ecrConfig.HTTPClient = factory.httpClient
	if endpointOverride != "" {
		ecrConfig.Endpoint = &endpointOverride
	}
	return ecrapi.New(session.New(&ecrConfig))
}
