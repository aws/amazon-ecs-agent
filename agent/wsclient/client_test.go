// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package wsclient

import (
	"io"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/gorilla/websocket"

	"github.com/stretchr/testify/assert"
)

const dockerEndpoint = "/var/run/docker.sock"

// TestConcurrentWritesDontPanic will force a panic in the websocket library if
// the implemented methods don't handle concurrency correctly
// See https://godoc.org/github.com/gorilla/websocket#hdr-Concurrency
func TestConcurrentWritesDontPanic(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, requests, _, _ := mockwsutils.StartMockServer(t, closeWS)
	defer mockServer.Close()

	req := ecsacs.AckRequest{Cluster: aws.String("test"), ContainerInstance: aws.String("test"), MessageId: aws.String("test")}

	cs := getClientServer(mockServer.URL)
	cs.Connect()

	executeTenRequests := func() {
		for i := 0; i < 10; i++ {
			cs.MakeRequest(&req)
		}
	}

	// Make requests from two separate routines to try and force a
	// concurrent write
	go executeTenRequests()
	executeTenRequests()

	t.Log("Waiting for all 20 requests to succeed")
	for i := 0; i < 20; i++ {
		<-requests
	}
}

// TestProxyVariableCustomValue ensures that a user is able to override the
// proxy variable by setting an environment variable
func TestProxyVariableCustomValue(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := mockwsutils.StartMockServer(t, closeWS)
	defer mockServer.Close()

	testString := "Custom no proxy string"
	os.Setenv("NO_PROXY", testString)
	getClientServer(mockServer.URL).Connect()

	assert.Equal(t, os.Getenv("NO_PROXY"), testString, "NO_PROXY should match user-supplied variable")
}

// TestProxyVariableDefaultValue verifies that NO_PROXY gets overridden if it
// isn't already set
func TestProxyVariableDefaultValue(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := mockwsutils.StartMockServer(t, closeWS)
	defer mockServer.Close()

	os.Unsetenv("NO_PROXY")
	getClientServer(mockServer.URL).Connect()

	expectedEnvVar := "169.254.169.254,169.254.170.2," + dockerEndpoint

	assert.Equal(t, os.Getenv("NO_PROXY"), expectedEnvVar, "Variable NO_PROXY expected to be overwritten when no default value supplied")
}

// TestHandleMessagePermissibleCloseCode ensures that permissible close codes
// are wrapped in io.EOF
func TestHandleMessagePermissibleCloseCode(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	messageError := make(chan error)
	mockServer, _, _, _, _ := mockwsutils.StartMockServer(t, closeWS)
	cs := getClientServer(mockServer.URL)
	cs.Connect()

	go func() {
		messageError <- cs.ConsumeMessages()
	}()

	closeWS <- websocket.FormatCloseMessage(websocket.CloseNormalClosure, ":)")
	assert.EqualError(t, <-messageError, io.EOF.Error(), "expected EOF for normal close code")
}

// TestHandleMessageUnexpectedCloseCode checks that unexpected close codes will
// be returned as is (not wrapped in io.EOF)
func TestHandleMessageUnexpectedCloseCode(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	messageError := make(chan error)
	mockServer, _, _, _, _ := mockwsutils.StartMockServer(t, closeWS)
	cs := getClientServer(mockServer.URL)
	cs.Connect()

	go func() {
		messageError <- cs.ConsumeMessages()
	}()

	closeWS <- websocket.FormatCloseMessage(websocket.CloseTryAgainLater, ":(")
	assert.True(t, websocket.IsCloseError(<-messageError, websocket.CloseTryAgainLater), "Expected error from websocket library")
}

func getClientServer(url string) *ClientServerImpl {
	types := []interface{}{ecsacs.AckRequest{}}

	return &ClientServerImpl{
		URL: url,
		AgentConfig: &config.Config{
			AcceptInsecureCert: true,
			AWSRegion:          "us-east-1",
			DockerEndpoint:     "unix://" + dockerEndpoint,
		},
		CredentialProvider: credentials.AnonymousCredentials,
		TypeDecoder:        BuildTypeDecoder(types),
	}
}
