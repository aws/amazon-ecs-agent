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

package wsclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock/utils"
	mock_wsconn "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/wsconn/mock"
	"github.com/golang/mock/gomock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/gorilla/websocket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dockerEndpoint                  = "/var/run/docker.sock"
	mockDisconnectTimeoutMetricName = "DisconnectTimeout"
)

// Close closes the underlying connection. Implement Close() in this file
// as ClientServerImpl doesn't implement it. This is needed by the
// TestSetReadDeadline* tests.
func (cs *ClientServerImpl) Close() error {
	return cs.Disconnect()
}

func TestClientProxy(t *testing.T) {
	proxy_url := "127.0.0.1:1234"
	os.Setenv("HTTP_PROXY", proxy_url)
	defer os.Unsetenv("HTTP_PROXY")

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer("http://www.amazon.com", types, 1)
	_, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), proxy_url), "proxy not found: %s", err.Error())
}

// TestConcurrentWritesDontPanic will force a panic in the websocket library if
// the implemented methods don't handle concurrency correctly.
// See https://godoc.org/github.com/gorilla/websocket#hdr-Concurrency.
func TestConcurrentWritesDontPanic(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, requests, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()
	defer mockServer.Close()

	var waitForRequests sync.WaitGroup
	waitForRequests.Add(1)

	go func() {
		for i := 0; i < 20; i++ {
			<-requests
		}
		waitForRequests.Done()
	}()
	req := ecsacs.AckRequest{Cluster: aws.String("test"), ContainerInstance: aws.String("test"), MessageId: aws.String("test")}

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	executeTenRequests := func() {
		for i := 0; i < 10; i++ {
			assert.NoError(t, cs.MakeRequest(&req))
		}
	}

	// Make requests from two separate routines to try and force a
	// concurrent write.
	go executeTenRequests()
	go executeTenRequests()

	t.Log("Waiting for all 20 requests to succeed")
	waitForRequests.Wait()
}

func getTestClientServer(url string, msgType []interface{}, rwTimeout time.Duration) *ClientServerImpl {
	testCreds := credentials.NewStaticCredentials("test-id", "test-secret", "test-token")

	return &ClientServerImpl{
		URL: url,
		Cfg: &WSClientMinAgentConfig{
			AcceptInsecureCert: true,
			AWSRegion:          "us-east-1",
			DockerEndpoint:     "unix://" + dockerEndpoint,
			IsDocker:           true,
		},
		CredentialProvider: testCreds,
		TypeDecoder:        BuildTypeDecoder(msgType),
		RWTimeout:          rwTimeout * time.Second,
		RequestHandlers:    make(map[string]RequestHandler),
		MetricsFactory:     metrics.NewNopEntryFactory(),
	}
}

// TestProxyVariableCustomValue ensures that a user is able to override the
// proxy variable by setting an environment variable.
func TestProxyVariableCustomValue(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()
	defer mockServer.Close()

	testString := "Custom no proxy string"
	os.Setenv("NO_PROXY", testString)
	types := []interface{}{ecsacs.AckRequest{}}
	timer, err := getTestClientServer(mockServer.URL, types, 1).Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	assert.Equal(t, os.Getenv("NO_PROXY"), testString, "NO_PROXY should match user-supplied variable")
}

// TestProxyVariableDefaultValue verifies that NO_PROXY gets overridden if it
// isn't already set.
func TestProxyVariableDefaultValue(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()
	defer mockServer.Close()

	os.Unsetenv("NO_PROXY")
	types := []interface{}{ecsacs.AckRequest{}}
	timer, err := getTestClientServer(mockServer.URL, types, 1).Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	expectedEnvVar := "169.254.169.254,169.254.170.2," + dockerEndpoint

	assert.Equal(t, os.Getenv("NO_PROXY"), expectedEnvVar, "Variable NO_PROXY expected to be overwritten when no default value supplied")
}

// TestHandleMessagePermissibleCloseCode ensures that permissible close codes
// are wrapped in io.EOF.
func TestHandleMessagePermissibleCloseCode(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	ctx := context.Background()
	messageError := make(chan error)
	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	assert.True(t, cs.IsReady(), "expected websocket connection to be ready")

	go func() {
		messageError <- cs.ConsumeMessages(ctx)
	}()

	closeWS <- websocket.FormatCloseMessage(websocket.CloseNormalClosure, ":)")
	assert.EqualError(t, <-messageError, io.EOF.Error(), "expected EOF for normal close code")
}

// TestHandleMessageUnexpectedCloseCode checks that unexpected close codes will
// be returned as is (not wrapped in io.EOF).
func TestHandleMessageUnexpectedCloseCode(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	messageError := make(chan error)
	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	assert.True(t, cs.IsReady(), "expected websocket connection to be ready")

	ctx := context.Background()
	go func() {
		messageError <- cs.ConsumeMessages(ctx)
	}()

	closeWS <- websocket.FormatCloseMessage(websocket.CloseTryAgainLater, ":(")
	assert.True(t, websocket.IsCloseError(<-messageError, websocket.CloseTryAgainLater), "Expected error from websocket library")
}

// TestHandlNonHTTPSEndpoint verifies that the wsclient can handle communication over
// an HTTP (so WS) connection.
func TestHandleNonHTTPSEndpoint(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, requests, _, _ := utils.GetMockServer(closeWS)
	mockServer.Start()
	defer mockServer.Close()

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	assert.True(t, cs.IsReady(), "expected websocket connection to be ready")

	req := ecsacs.AckRequest{Cluster: aws.String("test"), ContainerInstance: aws.String("test"), MessageId: aws.String("test")}
	cs.MakeRequest(&req)

	t.Log("Waiting for single request to be visible server-side")
	<-requests
}

// TestHandleIncorrectHttpScheme checks that an incorrect URL scheme results in
// an error.
func TestHandleIncorrectURLScheme(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()
	defer mockServer.Close()

	mockServerURL, _ := url.Parse(mockServer.URL)
	mockServerURL.Scheme = "notaparticularlyrealscheme"

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServerURL.String(), types, 1)
	_, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	assert.Error(t, err, "Expected error for incorrect URL scheme")
}

// TestWebsocketScheme checks that websocketScheme handles valid and invalid mappings
// correctly.
func TestWebsocketScheme(t *testing.T) {
	// Test valid schemes.
	validMappings := map[string]string{
		"http":  "ws",
		"https": "wss",
	}

	for input, expectedOutput := range validMappings {
		actualOutput, err := websocketScheme(input)

		assert.NoError(t, err, "Unexpected error for valid http scheme")
		assert.Equal(t, actualOutput, expectedOutput, "Valid http schemes should map to a websocket scheme")
	}

	// Test an invalid mapping.
	_, err := websocketScheme("highly-likely-to-be-junk")
	assert.Error(t, err, "Expected error for invalid http scheme")
}

func TestSetReadDeadlineClosedConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := &ClientServerImpl{conn: conn}

	ctx := context.Background()
	opErr := &net.OpError{Err: errors.New(errClosed)}
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(opErr)
	assert.EqualError(t, cs.ConsumeMessages(ctx), opErr.Error())
}

func TestSetReadDeadlineError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := &ClientServerImpl{conn: conn}
	ctx := context.Background()
	gomock.InOrder(
		conn.EXPECT().SetReadDeadline(gomock.Any()).Return(errors.New("error")),
		conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil),
		conn.EXPECT().Close().Return(nil),
	)
	assert.Error(t, cs.ConsumeMessages(ctx))
}

// TestAddRequestPayloadHandler tests adding a request handler to client.
// The test also expects the message consumed to be handled by correct handler.
func TestAddRequestPayloadHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(websocket.TextMessage,
		[]byte(`{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn"}]}}`),
		nil).MinTimes(1)
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	closeWS := make(chan []byte)
	defer close(closeWS)

	ctx := context.Background()
	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.PayloadMessage{}}
	messageError := make(chan error)
	cs := getTestClientServer(mockServer.URL, types, 1)
	cs.conn = conn

	defer cs.Close()

	messageChannel := make(chan *ecsacs.PayloadMessage)
	reqHandler := func(payload *ecsacs.PayloadMessage) {
		messageChannel <- payload
	}
	cs.AddRequestHandler(reqHandler)

	go func() {
		messageError <- cs.ConsumeMessages(ctx)
		cs.Close()
	}()

	expectedMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{{
			Arn: aws.String("arn"),
		}},
	}

	assert.Equal(t, expectedMessage, <-messageChannel)

}

// TestMakeUnrecognizedRequest tests if the correct error type is returned
// on unrecognized request type.
func TestMakeUnrecognizedRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.PayloadMessage{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	cs.conn = conn

	defer cs.Close()

	err := cs.MakeRequest(t)
	if _, ok := err.(*UnrecognizedWSRequestType); !ok {
		t.Fatal("Expected unrecognized request type")
	}
}

// TestWriteCloseMessage tests if the wsclient can successfully close the connection
// and write close message. The close message is expected to be received on server side.
func TestWriteCloseMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, errChan, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.PayloadMessage{}}
	cs := getTestClientServer(mockServer.URL, types, 1)
	cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)

	defer cs.Close()

	err := cs.WriteCloseMessage()
	assert.NoError(t, err)
	assert.Error(t, <-errChan)
}

// TestCtxCancel tests if the passed context, on receiving the cancel
// on the created ctx.Done channel, performs the expected behavior of
// closing the connection and returns the ctx error.
func TestCtxCancel(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	ctx, cancel := context.WithCancel(context.Background())
	messageError := make(chan error)
	mockServer, _, _, _, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	types := []interface{}{ecsacs.AckRequest{}}
	cs := getTestClientServer(mockServer.URL, types, 2)
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, DisconnectTimeout, DisconnectJitterMax)
	require.NoError(t, err)
	defer timer.Stop()

	assert.True(t, cs.IsReady(), "expected websocket connection to be ready")

	go func() {
		messageError <- cs.ConsumeMessages(ctx)
	}()
	// Cancel the context.
	cancel()
	err = <-messageError
	assert.Equal(t, err.Error(), context.Canceled.Error(), "Context canceled error expected.")
}

func TestPeriodicDisconnect(t *testing.T) {
	closeWS := make(chan []byte)
	defer close(closeWS)

	mockServer, _, _, errChan, _ := utils.GetMockServer(closeWS)
	mockServer.StartTLS()

	messageError := make(chan error)
	ctx := context.Background()
	types := []interface{}{ecsacs.AckRequest{}}

	// Setting a higher rwtimeout to allow disconnect due to periodic timeouts.
	cs := getTestClientServer(mockServer.URL, types, 20)
	// Setting up a lower disconnect timer value for testing.
	disconnectTimeout := 10 * time.Second
	disconnectTimeoutJitter := 2 * time.Second
	timer, err := cs.Connect(mockDisconnectTimeoutMetricName, disconnectTimeout, disconnectTimeoutJitter)
	require.NoError(t, err)
	defer timer.Stop()
	assert.True(t, cs.IsReady(), "expected websocket connection to be ready")

	// Using time difference to ensure that the disconnect was
	go func() {
		startTime := time.Now()
		messageError <- cs.ConsumeMessages(ctx)
		endTime := time.Now()
		timeDiff := endTime.Sub(startTime)
		assert.True(t, timeDiff >= disconnectTimeout,
			"ConsumeMessages should be not be closed before disconnectTimeout has elapsed.")
	}()

	// Assert that the connection is closed on the server side as expected
	assert.EqualError(t, <-errChan, "websocket: close 1000 (normal): ConnectionExpired: Reconnect to continue")
	// Assert that the connection is closed on the client side as expected
	assert.EqualError(t, <-messageError, io.EOF.Error(), "expected EOF for normal close code")
}
