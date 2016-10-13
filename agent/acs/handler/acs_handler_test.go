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
package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	rolecredentials "github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/gorilla/websocket"

	"github.com/golang/mock/gomock"
)

const (
	samplePayloadMessage = `
{
  "type": "PayloadMessage",
  "message": {
    "messageId": "123",
    "tasks": [
      {
        "taskDefinitionAccountId": "123",
        "containers": [
          {
            "environment": {},
            "name": "name",
            "cpu": 1,
            "essential": true,
            "memory": 1,
            "portMappings": [],
            "overrides": "{}",
            "image": "i",
            "mountPoints": [],
            "volumesFrom": []
          }
        ],
        "roleCredentials": {
          "credentialsId": "credsId",
          "accessKeyId": "accessKeyId",
          "expiration": "2016-03-25T06:17:19.318+0000",
          "roleArn": "r1",
          "secretAccessKey": "secretAccessKey",
          "sessionToken": "token"
        },
        "version": "3",
        "volumes": [],
        "family": "f",
        "arn": "arn",
        "desiredStatus": "RUNNING"
      }
    ],
    "generatedAt": 1,
    "clusterArn": "1",
    "containerInstanceArn": "1",
    "seqNum": 1
  }
}
`
	sampleRefreshCredentialsMessage = `
{
  "type": "IAMRoleCredentialsMessage",
  "message": {
    "messageId": "123",
    "clusterArn": "default",
    "taskArn": "t1",
    "roleCredentials": {
      "credentialsId": "credsId",
      "accessKeyId": "newakid",
      "expiration": "later",
      "roleArn": "r1",
      "secretAccessKey": "newskid",
      "sessionToken": "newstkn"
    }
  }
}
`
	acsURL = "http://endpoint.tld"
)

type mockSession struct {
	client wsclient.ClientServer
}

func (m *mockSession) createACSClient(url string) wsclient.ClientServer {
	return m.client
}

func (m *mockSession) connectedToACS() {
}

func (m *mockSession) getSendCredentialsURLParameter() string {
	return "true"
}
func TestAcsWsUrl(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)

	taskEngine.EXPECT().Version().Return("Docker version result", nil)

	wsurl := acsWsURL(acsURL, "myCluster", "myContainerInstance", taskEngine, &mockSession{})

	parsed, err := url.Parse(wsurl)
	if err != nil {
		t.Fatal("Should be able to parse url")
	}

	if parsed.Path != "/ws" {
		t.Fatal("Wrong path")
	}

	if parsed.Query().Get("clusterArn") != "myCluster" {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("containerInstanceArn") != "myContainerInstance" {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("agentVersion") != version.Version {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("agentHash") != version.GitHashString() {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("dockerVersion") != "Docker version result" {
		t.Fatal("Wrong docker version")
	}
	if parsed.Query().Get(sendCredentialsURLParameterName) != "true" {
		t.Fatalf("Wrong value set for: %s", sendCredentialsURLParameterName)
	}
	if parsed.Query().Get("seqNum") != "1" {
		t.Fatal("Wrong seqNum")
	}
}

// TestHandlerReconnectsOnConnectErrors tests if handler reconnects retries
// to establish the session with ACS when ClientServer.Connect() returns errors
func TestHandlerReconnectsOnConnectErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ecsClient := mock_api.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	statemanager := statemanager.NewNoopStateManager()

	ctx, cancel := context.WithCancel(context.Background())
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		// Connect fails 10 times
		mockWsClient.EXPECT().Connect().Return(io.EOF).Times(10),
		// Cancel trying to connect to ACS on the 11th attempt
		// Failure to retry on Connect() errors should cause the
		// test  to time out as the context is never cancelled
		mockWsClient.EXPECT().Connect().Do(func() {
			cancel()
		}).Return(io.EOF),
	)
	session := &mockSession{mockWsClient}

	args := StartSessionArguments{
		ContainerInstanceArn: "myArn",
		CredentialProvider:   credentials.AnonymousCredentials,
		Config:               &config.Config{Cluster: "someCluster"},
		TaskEngine:           taskEngine,
		ECSClient:            ecsClient,
		StateManager:         statemanager,
		AcceptInvalidCert:    true,
		_heartbeatTimeout:    20 * time.Millisecond,
		_heartbeatJitter:     10 * time.Millisecond,
	}
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	go func() {
		startSession(ctx, args, backoff, session)
	}()

	// Wait for context to be cancelled
	select {
	case <-ctx.Done():
	}
}

// TestHandlerReconnectsOnServeErrors tests if the handler retries to
// to establish the session with ACS when ClientServer.Connect() returns errors
func TestHandlerReconnectsOnServeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ecsClient := mock_api.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	statemanager := statemanager.NewNoopStateManager()

	ctx, cancel := context.WithCancel(context.Background())
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		// Serve fails 10 times
		mockWsClient.EXPECT().Serve().Return(io.EOF).Times(10),
		// Cancel trying to Serve ACS requests on the 11th attempt
		// Failure to retry on Serve() errors should cause the
		// test to time out as the context is never cancelled
		mockWsClient.EXPECT().Serve().Do(func() {
			cancel()
		}).Return(io.EOF),
	)
	session := &mockSession{mockWsClient}

	args := StartSessionArguments{
		ContainerInstanceArn: "myArn",
		CredentialProvider:   credentials.AnonymousCredentials,
		Config:               &config.Config{Cluster: "someCluster"},
		TaskEngine:           taskEngine,
		ECSClient:            ecsClient,
		StateManager:         statemanager,
		AcceptInvalidCert:    true,
		_heartbeatTimeout:    20 * time.Millisecond,
		_heartbeatJitter:     10 * time.Millisecond,
	}
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	go func() {
		startSession(ctx, args, backoff, session)
	}()

	// Wait for context to be cancelled
	select {
	case <-ctx.Done():
	}
}

// TestHandlerReconnectsOnDiscoverPollEndpointError tests if handler retries
// to establish the session with ACS on DiscoverPollEndpoint errors
func TestHandlerReconnectsOnDiscoverPollEndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Serve().Do(func() {
		// Serve() cancels the context
		cancel()
	}).Return(io.EOF)

	session := &mockSession{mockWsClient}

	gomock.InOrder(
		// DiscoverPollEndpoint returns an error on its first invocation
		ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return("", fmt.Errorf("oops")).Times(1),
		// Second invocation returns a success
		ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).Times(1),
	)
	args := StartSessionArguments{
		ContainerInstanceArn: "myArn",
		CredentialProvider:   credentials.AnonymousCredentials,
		Config:               &config.Config{Cluster: "someCluster"},
		TaskEngine:           taskEngine,
		ECSClient:            ecsClient,
		StateManager:         statemanager,
		AcceptInvalidCert:    true,
		_heartbeatTimeout:    20 * time.Millisecond,
		_heartbeatJitter:     10 * time.Millisecond,
	}
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	go func() {
		startSession(ctx, args, backoff, session)
	}()
	start := time.Now()

	// Wait for context to be cancelled
	select {
	case <-ctx.Done():
	}

	// Measure the duration between retries
	timeSinceStart := time.Since(start)
	if timeSinceStart < connectionBackoffMin {
		t.Errorf("Duration since start is less than minimum threshold for backoff: %s", timeSinceStart.String())
	}

	// The upper limit here should really be connectionBackoffMin + (connectionBackoffMin * jitter)
	// But, it can be off by a few milliseconds to account for execution of other instructions
	// In any case, it should never be higher than 2*connectionBackoffMin
	if timeSinceStart > 2*connectionBackoffMin {
		t.Errorf("Duration since start is greater than maximum anticipated wait time: %v", timeSinceStart.String())
	}

}

// TestConnectionIsClosedOnIdle tests if the connection to ACS is closed
// when the channel is idle
func TestConnectionIsClosedOnIdle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).Do(func(v interface{}) {}).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).Do(func(v interface{}) {}).AnyTimes()
	mockWsClient.EXPECT().Connect().Return(nil)
	mockWsClient.EXPECT().Serve().Do(func() {
		// Pretend as if the maximum heartbeatTimeout duration has
		// been breached while Serving requests
		time.Sleep(30 * time.Millisecond)
	}).Return(io.EOF)

	connectionClosed := make(chan bool)
	mockWsClient.EXPECT().Close().Do(func() {
		// Record connection closed
		connectionClosed <- true
	}).Return(nil)
	ctx := context.Background()
	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	args := StartSessionArguments{
		ContainerInstanceArn: "myArn",
		CredentialProvider:   credentials.AnonymousCredentials,
		Config:               &config.Config{Cluster: "someCluster"},
		TaskEngine:           taskEngine,
		ECSClient:            ecsClient,
		StateManager:         statemanager,
		AcceptInvalidCert:    true,
		_heartbeatTimeout:    20 * time.Millisecond,
		_heartbeatJitter:     10 * time.Millisecond,
	}
	go func() {
		timer := newDisconnectionTimer(mockWsClient, args.time(), args.heartbeatTimeout(), args.heartbeatJitter())
		defer timer.Stop()
		startACSSession(ctx, mockWsClient, timer, args, backoff, &mockSession{})
	}()

	// Wait for connection to be closed. If the connection is not closed
	// due to inactivity, the test will time out
	<-connectionClosed
}

func TestHandlerDoesntLeakGouroutines(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	closeWS := make(chan bool)
	server, serverIn, requests, errs, err := startMockAcsServer(t, closeWS)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			select {
			case <-requests:
			case <-errs:
			}
		}
	}()

	timesConnected := 0
	ecsClient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).AnyTimes().Do(func(_ interface{}) {
		timesConnected++
	})
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()
	taskEngine.EXPECT().AddTask(gomock.Any()).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan bool, 1)
	go func() {
		StartSession(ctx, StartSessionArguments{
			ContainerInstanceArn: "myArn",
			CredentialProvider:   credentials.AnonymousCredentials,
			Config:               &config.Config{Cluster: "someCluster"},
			TaskEngine:           taskEngine,
			ECSClient:            ecsClient,
			StateManager:         statemanager,
			AcceptInvalidCert:    true,
			CredentialsManager:   rolecredentials.NewManager(),
			_heartbeatTimeout:    20 * time.Millisecond,
			_heartbeatJitter:     10 * time.Millisecond,
		})
		ended <- true
	}()
	// Warm it up
	serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true}}`
	serverIn <- samplePayloadMessage

	beforeGoroutines := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true}}`
		serverIn <- samplePayloadMessage
		closeWS <- true
	}

	cancel()
	<-ended

	afterGoroutines := runtime.NumGoroutine()

	t.Logf("Gorutines after 1 and after 100 acs messages: %v and %v", beforeGoroutines, afterGoroutines)

	if timesConnected < 50 {
		t.Fatal("Expected times connected to be a large number, was ", timesConnected)
	}
	if afterGoroutines > beforeGoroutines+5 {
		t.Error("Goroutine leak, oh no!")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	}
}

// TestStartSessionHandlesRefreshCredentialsMessages tests the agent restart
// scenario where the payload to refresh credentials is processed immediately on
// connection establishment with ACS
func TestStartSessionHandlesRefreshCredentialsMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	closeWS := make(chan bool)
	server, serverIn, requestsChan, errChan, err := startMockAcsServer(t, closeWS)
	if err != nil {
		t.Fatal(err)
	}
	defer close(serverIn)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-requestsChan:
				// Cancel the context when we get the ack request
				cancel()
			}
		}
	}()

	// DiscoverPollEndpoint returns the URL for the server that we started
	ecsClient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).Times(1)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	credentialsManager := mock_credentials.NewMockManager(ctrl)

	ended := make(chan bool, 1)
	go func() {
		StartSession(ctx, StartSessionArguments{
			ContainerInstanceArn: "myArn",
			CredentialProvider:   credentials.AnonymousCredentials,
			Config:               &config.Config{Cluster: "someCluster"},
			TaskEngine:           taskEngine,
			ECSClient:            ecsClient,
			StateManager:         stateManager,
			AcceptInvalidCert:    true,
			CredentialsManager:   credentialsManager,
		})
		// StartSession should never return unless the context is canceled
		ended <- true
	}()

	updatedCredentials := rolecredentials.TaskIAMRoleCredentials{}
	taskFromEngine := &api.Task{}
	credentialsIdInRefreshMessage := "credsId"
	// Ensure that credentials manager interface methods are invoked in the
	// correct order, with expected arguments
	gomock.InOrder(
		// Return a task from the engine for GetTaskByArn
		taskEngine.EXPECT().GetTaskByArn("t1").Return(taskFromEngine, true),
		// The last invocation of SetCredentials is to update
		// credentials when a refresh message is recieved by the handler
		credentialsManager.EXPECT().SetTaskCredentials(gomock.Any()).Do(func(creds rolecredentials.TaskIAMRoleCredentials) {
			updatedCredentials = creds
			// Validate parsed credentials after the update
			expectedCreds := rolecredentials.TaskIAMRoleCredentials{
				ARN: "t1",
				IAMRoleCredentials: rolecredentials.IAMRoleCredentials{
					RoleArn:         "r1",
					AccessKeyID:     "newakid",
					SecretAccessKey: "newskid",
					SessionToken:    "newstkn",
					Expiration:      "later",
					CredentialsID:   credentialsIdInRefreshMessage,
				},
			}
			if !reflect.DeepEqual(updatedCredentials, expectedCreds) {
				t.Errorf("Mismatch between expected and credentials expected: %v, added: %v", expectedCreds, updatedCredentials)
			}
		}).Return(nil),
	)
	serverIn <- sampleRefreshCredentialsMessage

	select {
	case err := <-errChan:
		t.Fatal("Error should not have been returned from server", err)
	case <-ctx.Done():
		// Context is canceled when requestsChan recieves an ack
	}

	// Validate that the correct credentialsId is set for the task
	credentialsIdFromTask := taskFromEngine.GetCredentialsId()
	if credentialsIdFromTask != credentialsIdInRefreshMessage {
		t.Errorf("Mismatch between expected and added credentials id for task, expected: %s, aded: %s", credentialsIdInRefreshMessage, credentialsIdFromTask)
	}

	go server.Close()
	// Cancel context should close the session
	<-ended
}

// TestACSSessionResourcesCorrectlySetsSendCredentials tests if acsSessionResources
// struct correctly sets 'sendCredentials'
func TestACSSessionResourcesCorrectlySetsSendCredentials(t *testing.T) {
	acsResources := newSessionResources(StartSessionArguments{})
	// Validate that 'sendCredentials' is set to true on create
	sendCredentials := acsResources.getSendCredentialsURLParameter()
	if sendCredentials != "true" {
		t.Errorf("Mismatch in sendCredentials URL parameter value, expected: 'true', got: %s", sendCredentials)
	}
	// Simulate a successful connection to ACS
	acsResources.connectedToACS()
	// On successful connection to ACS, 'sendCredentials' should be set to false
	sendCredentials = acsResources.getSendCredentialsURLParameter()
	if sendCredentials != "false" {
		t.Errorf("Mismatch in sendCredentials URL parameter value, expected: 'false', got: %s", sendCredentials)
	}
}

// TestHandlerReconnectsCorrectlySetsSendCredentialsURLParameter tests if
// the 'sendCredentials' URL parameter is set correctly for successive
// invocations of startACSSession
func TestHandlerReconnectsCorrectlySetsSendCredentialsURLParameter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	ctx, cancel := context.WithCancel(context.Background())
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)

	args := StartSessionArguments{
		ContainerInstanceArn: "myArn",
		CredentialProvider:   credentials.AnonymousCredentials,
		Config:               &config.Config{Cluster: "someCluster"},
		TaskEngine:           taskEngine,
		ECSClient:            ecsClient,
		StateManager:         statemanager,
		AcceptInvalidCert:    true,
		_heartbeatTimeout:    20 * time.Millisecond,
		_heartbeatJitter:     10 * time.Millisecond,
	}
	session := newSessionResources(args)

	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Serve().Return(io.EOF).AnyTimes()
	gomock.InOrder(
		// When the websocket client connects to ACS for the first
		// time, 'sendCredentials' should be set to true
		mockWsClient.EXPECT().Connect().Do(func() {
			validateSendCredentialsInSession(t, session, "true")
		}).Return(nil),
		// For all subsequent connections to ACS, 'sendCredentials'
		// should be set to false
		mockWsClient.EXPECT().Connect().Do(func() {
			validateSendCredentialsInSession(t, session, "false")
		}).Return(nil).AnyTimes(),
	)

	backoff := utils.NewSimpleBackoff(connectionBackoffMin, connectionBackoffMax, connectionBackoffJitter, connectionBackoffMultiplier)
	timer := newDisconnectionTimer(mockWsClient, args.time(), args.heartbeatTimeout(), args.heartbeatJitter())
	defer timer.Stop()
	go func() {
		for i := 0; i < 10; i++ {
			startACSSession(ctx, mockWsClient, timer, args, backoff, session)
		}
		cancel()
	}()

	// Wait for context to be cancelled
	select {
	case <-ctx.Done():
	}
}

func startMockAcsServer(t *testing.T, closeWS <-chan bool) (*httptest.Server, chan<- string, <-chan string, <-chan error, error) {
	serverChan := make(chan string, 1)
	requestsChan := make(chan string, 1)
	errChan := make(chan error, 1)

	serverRestart := make(chan bool, 1)
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		go func() {
			<-closeWS
			ws.WriteMessage(websocket.CloseMessage, nil)
			ws.Close()
			serverRestart <- true
			errChan <- io.EOF
		}()
		if err != nil {
			errChan <- err
		}
		go func() {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
			} else {
				requestsChan <- string(msg)
			}
		}()
		for {
			select {
			case str := <-serverChan:
				err := ws.WriteMessage(websocket.TextMessage, []byte(str))
				if err != nil {
					errChan <- err
				}
			case <-serverRestart:
				// Quit listening to serverChan if we've been closed
				return
			}
		}
	})

	server := httptest.NewTLSServer(handler)
	return server, serverChan, requestsChan, errChan, nil
}

// validateAddedTask validates fields in addedTask for expected values
// It returns an error if there's a mismatch
func validateAddedTask(expectedTask api.Task, addedTask api.Task) error {
	// The ecsacs.Task -> api.Task conversion initializes all fields in api.Task
	// with empty objects. So, we create a new object to compare with only those
	// fields that we are intrested in for comparison
	taskToCompareFromAdded := api.Task{
		Arn:                 addedTask.Arn,
		Family:              addedTask.Family,
		Version:             addedTask.Version,
		DesiredStatus:       addedTask.DesiredStatus,
		StartSequenceNumber: addedTask.StartSequenceNumber,
	}

	if !reflect.DeepEqual(expectedTask, taskToCompareFromAdded) {
		return fmt.Errorf("Mismatch between added and expected task: expected: %v, added: %v", expectedTask, taskToCompareFromAdded)
	}

	return nil
}

// validateAddedContainer validates fields in addedContainer for expected values
// It returns an error if there's a mismatch
func validateAddedContainer(expectedContainer *api.Container, addedContainer *api.Container) error {
	// The ecsacs.Task -> api.Task conversion initializes all fields in api.Container
	// with empty objects. So, we create a new object to compare with only those
	// fields that we are intrested in for comparison
	containerToCompareFromAdded := &api.Container{
		Name:      addedContainer.Name,
		Cpu:       addedContainer.Cpu,
		Essential: addedContainer.Essential,
		Memory:    addedContainer.Memory,
		Image:     addedContainer.Image,
	}
	if !reflect.DeepEqual(expectedContainer, containerToCompareFromAdded) {
		return fmt.Errorf("Mismatch between added and expected container: expected: %v, added: %v", expectedContainer, containerToCompareFromAdded)
	}
	return nil
}

func validateSendCredentialsInSession(t *testing.T, state sessionState, expected string) {
	sendCredentials := state.getSendCredentialsURLParameter()
	if sendCredentials != expected {
		t.Errorf("Incorrect value set for sendCredentials, expected: %s, got: %s", expected, sendCredentials)
	}
}
