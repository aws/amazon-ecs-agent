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

package httpclient

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/cipher"

	"github.com/stretchr/testify/assert"
)

const (
	testAgentVersion     = "Amazon ECS Agent - v0.0.0 (fffffff)"
	testOSType           = "linux"
	testDetailedOSFamily = "debian_11"
	testContainerName    = "test-container"
	testHTTPURL          = "http://amazon.com"
)

func TestNewHttpClient(t *testing.T) {
	expectedResult := New(time.Duration(10), true, testAgentVersion, testOSType, testDetailedOSFamily)
	transport := expectedResult.Transport.(*ecsRoundTripper)
	assert.Equal(t, cipher.SupportedCipherSuites, transport.transport.(*http.Transport).TLSClientConfig.CipherSuites)
	assert.Equal(t, true, transport.transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestNewHttpClientProxy(t *testing.T) {
	proxy_url := "127.0.0.1:1234"
	// Set Proxy
	os.Setenv("HTTP_PROXY", proxy_url)
	defer os.Unsetenv("HTTP_PROXY")

	client := New(time.Duration(10*time.Second), true, testAgentVersion, testOSType, testDetailedOSFamily)
	_, err := client.Get(testHTTPURL)
	// Client won't be able to connect because we have given a arbitrary proxy
	assert.Error(t, err)
	// Error message should contain the proxy url which shows that client tried to use the proxy url to connect
	assert.True(t, strings.Contains(err.Error(), proxy_url), "proxy url not found in: %s", err.Error())
}

func TestUserAgent(t *testing.T) {
	testCases := []struct {
		name                 string
		agentVersion         string
		osType               string
		detailedOSFamily     string
		contextContainerName string
		expectedUserAgent    string
	}{
		{
			name:              "basic user agent with detailed OS family",
			agentVersion:      testAgentVersion,
			osType:            testOSType,
			detailedOSFamily:  testDetailedOSFamily,
			expectedUserAgent: "Amazon ECS Agent - v0.0.0 (fffffff) (linux; debian_11) (+http://aws.amazon.com/ecs/)",
		},
		{
			name:              "basic user agent without detailed OS family",
			agentVersion:      testAgentVersion,
			osType:            testOSType,
			detailedOSFamily:  "",
			expectedUserAgent: "Amazon ECS Agent - v0.0.0 (fffffff) (linux) (+http://aws.amazon.com/ecs/)",
		},
		{
			name:                 "user agent with container name and detailed OS family",
			agentVersion:         testAgentVersion,
			osType:               testOSType,
			detailedOSFamily:     testDetailedOSFamily,
			contextContainerName: testContainerName,
			expectedUserAgent:    "Amazon ECS Agent - v0.0.0 (fffffff) (linux; debian_11) (+http://aws.amazon.com/ecs/) container/test-container",
		},
		{
			name:                 "user agent with container name, no detailed OS family",
			agentVersion:         testAgentVersion,
			osType:               testOSType,
			detailedOSFamily:     "",
			contextContainerName: testContainerName,
			expectedUserAgent:    "Amazon ECS Agent - v0.0.0 (fffffff) (linux) (+http://aws.amazon.com/ecs/) container/test-container",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripper := &ecsRoundTripper{
				agentVersion:     tc.agentVersion,
				osType:           tc.osType,
				detailedOSFamily: tc.detailedOSFamily,
			}

			// Create request with or without container name in context
			req, _ := http.NewRequest("GET", testHTTPURL, nil)
			if tc.contextContainerName != "" {
				ctx := context.WithValue(req.Context(), ContainerNameKey, tc.contextContainerName)
				req = req.WithContext(ctx)
			}

			userAgent := roundTripper.userAgent(req)
			assert.Equal(t, tc.expectedUserAgent, userAgent)
		})
	}
}
