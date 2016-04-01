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

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/gorilla/websocket"

	"github.com/golang/mock/gomock"
)

const samplePayloadMessage = `{"type":"PayloadMessage","message":{"messageId":"123","tasks":[{"taskDefinitionAccountId":"123","containers":[{"environment":{},"name":"name","cpu":1,"essential":true,"memory":1,"portMappings":[],"overrides":"{}","image":"i","mountPoints":[],"volumesFrom":[]}],"version":"3","volumes":[],"family":"f","arn":"arn","desiredStatus":"RUNNING"}],"generatedAt":1,"clusterArn":"1","containerInstanceArn":"1","seqNum":1}}`

func TestAcsWsUrl(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)

	taskEngine.EXPECT().Version().Return("Docker version result", nil)

	wsurl := AcsWsUrl("http://endpoint.tld", "myCluster", "myContainerInstance", taskEngine)

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
}

func TestHandlerReconnects(t *testing.T) {
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

	ecsClient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).Times(10)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

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
		})
		// This should never return
		ended <- true
	}()
	start := time.Now()
	for i := 0; i < 10; i++ {
		serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true}}`
		closeWS <- true
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Test took longer than expected; backoff should not have occured for EOF")
	}

	select {
	case <-ended:
		t.Fatal("Should not have stopped session")
	default:
	}
	cancel()
	<-ended
}

func TestHeartbeatOnlyWhenIdle(t *testing.T) {
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	closeWS := make(chan bool)
	server, serverIn, requestsChan, errChan, err := startMockAcsServer(t, closeWS)
	defer close(serverIn)

	go func() {
		for {
			<-requestsChan
		}
	}()
	if err != nil {
		t.Fatal(err)
	}

	// We're testing that it does not reconnect here; must be the case
	ecsClient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).Times(1)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

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
		})
		ended <- true
	}()

	taskAdded := make(chan bool)
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(interface{}) {
		taskAdded <- true
	}).Times(10)
	for i := 0; i < 10; i++ {
		serverIn <- samplePayloadMessage
		testTime.Warp(1 * time.Minute)
		<-taskAdded
	}

	select {
	case <-ended:
		t.Fatal("Should not have stop session")
	case err := <-errChan:
		t.Fatal("Error should not have been returned from server", err)
	default:
	}
	go server.Close()
	cancel()
	<-ended
}

func TestHandlerDoesntLeakGouroutines(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)

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
		StartSession(ctx, StartSessionArguments{"myArn", credentials.AnonymousCredentials, &config.Config{Cluster: "someCluster"}, taskEngine, ecsClient, statemanager, true})
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
	testTime.Cancel()
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

// TestHandlePayloadMessageWithNoMessageId tests that agent doesn't ack payload messages
// that do not contain message ids
func TestHandlePayloadMessageWithNoMessageId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	ackBuffer := make(chan string, payloadMessageBufferSize)

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
	}

	// test adding a payload message without the MessageId field
	err := handlePayloadMessage(ackBuffer, payloadMessage, taskEngine, ecsClient, stateManager)
	if err == nil {
		t.Error("Expected error while adding a task with no message id")
	}

	// test adding a payload message with blank MessageId
	payloadMessage.MessageId = aws.String("")
	err = handlePayloadMessage(ackBuffer, payloadMessage, taskEngine, ecsClient, stateManager)

	if err == nil {
		t.Error("Expected error while adding a task with no message id")
	}

}

// TestHandlePayloadMessageAddTaskError tests that agent does not ack payload messages
// when task engine fails to add tasks
func TestHandlePayloadMessageAddTaskError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	// Return error from AddTask
	taskEngine.EXPECT().AddTask(gomock.Any()).Return(fmt.Errorf("oops")).Times(1)

	ackBuffer := make(chan string, payloadMessageBufferSize)
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String("123"),
	}
	err := handlePayloadMessage(ackBuffer, payloadMessage, taskEngine, ecsClient, stateManager)

	if err == nil {
		t.Error("Expected error while adding the task")
	}

}

// TestHandlePayloadMessageStateSaveError tests that agent does not ack payload messages
// when state saver fails to save state
func TestHandlePayloadMessageStateSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)

	taskEngine := engine.NewMockTaskEngine(ctrl)

	// Save added task in the addedTask variable
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	// State manager returns error on save
	stateManager := mock_statemanager.NewMockStateManager(ctrl)
	stateManager.EXPECT().Save().Return(fmt.Errorf("oops"))

	ackBuffer := make(chan string, payloadMessageBufferSize)
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String("123"),
	}
	err := handlePayloadMessage(ackBuffer, payloadMessage, taskEngine, ecsClient, stateManager)

	if err == nil {
		t.Error("Expected error while adding a task with no message id")
	}

	// We expect task to be added to the engine even though it hasn't been saved
	expectedTask := &api.Task{
		Arn: "t1",
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestHandlePayloadMessageAckedWhenTaskAdded tests if the handler generates an ack
// after processing a payload message.
func TestHandlePayloadMessageAckedWhenTaskAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())

	taskEngine := engine.NewMockTaskEngine(ctrl)
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	// go routine to wait for message id ack from ackBuffer
	ackBuffer := make(chan string, payloadMessageBufferSize)
	var messageIdFromAck string
	waitForAck := make(chan struct{})
	go func() {
	Loop:
		for {
			select {
			case messageIdFromAck = <-ackBuffer:
				cancel()
			case <-ctx.Done():
				break Loop
			}
		}
		waitForAck <- struct{}{}
	}()

	// Send a payload message
	messageId := "123"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String(messageId),
	}
	err := handlePayloadMessage(ackBuffer, payloadMessage, taskEngine, ecsClient, stateManager)
	if err != nil {
		t.Errorf("Error handling payload message: %v", err)
	}

	// Wait till we get an ack from the ackBuffer
	select {
	case <-waitForAck:
	}

	// Verify the message id acked
	if messageIdFromAck != messageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", messageId, messageIdFromAck)
	}

	// Verify if task added == expected task
	expectedTask := &api.Task{
		Arn: "t1",
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks tests if tasks with desired status
// 'RUNNING' are added after tasks with desired status 'STOPPED'
func TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	taskEngine := engine.NewMockTaskEngine(ctrl)

	var tasksAddedToEngine []*api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		tasksAddedToEngine = append(tasksAddedToEngine, task)
	}).Times(2)

	messageId := "123"
	stoppedTaskArn := "stoppedTask"
	runningTaskArn := "runningTask"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn:           aws.String(runningTaskArn),
				DesiredStatus: aws.String("RUNNING"),
			},
			&ecsacs.Task{
				Arn:           aws.String(stoppedTaskArn),
				DesiredStatus: aws.String("STOPPED"),
			},
		},
		MessageId: aws.String(messageId),
	}
	ok := addPayloadTasks(ecsClient, payloadMessage, taskEngine)
	if !ok {
		t.Error("addPayloadTasks returned false")
	}
	if len(tasksAddedToEngine) != 2 {
		t.Errorf("Incorrect number of tasks added to the engine. Expected: %d, got: %d", 2, len(tasksAddedToEngine))
	}

	// Verify if stopped task is added before running task
	firstTaskAdded := tasksAddedToEngine[0]
	if firstTaskAdded.Arn != stoppedTaskArn {
		t.Errorf("Expected first task arn: %s, got: %s", stoppedTaskArn, firstTaskAdded.Arn)
	}
	if firstTaskAdded.DesiredStatus != api.TaskStopped {
		t.Errorf("Expected first task state be be: %s , got: %s", "STOPPED", firstTaskAdded.DesiredStatus.String())
	}

	secondTaskAdded := tasksAddedToEngine[1]
	if secondTaskAdded.Arn != runningTaskArn {
		t.Errorf("Expected second task arn: %s, got: %s", runningTaskArn, secondTaskAdded.Arn)
	}
	if secondTaskAdded.DesiredStatus != api.TaskRunning {
		t.Errorf("Expected second task state be be: %s , got: %s", "RUNNNING", secondTaskAdded.DesiredStatus.String())
	}
}

// TestPayloadBufferHandler tests if the async payloadBufferHandler routine
// acks messages after adding tasks
func TestPayloadBufferHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())

	payloadBuffer := make(chan *ecsacs.PayloadMessage, payloadMessageBufferSize)
	ackBuffer := make(chan string, payloadMessageBufferSize)
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)
	go payloadBufferHandler(payloadBuffer, ackBuffer, taskEngine, ecsClient, stateManager, ctx)

	// Send a payload message to the payloadBufferChannel
	messageId := "123"
	taskArn := "t1"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String(taskArn),
			},
		},
		MessageId: aws.String(messageId),
	}
	payloadBuffer <- payloadMessage

	// Wait till we get an ack
	var messageIdFromAck string
Loop:
	for {
		select {
		case messageIdFromAck = <-ackBuffer:
			cancel()
		case <-ctx.Done():
			break Loop
		}
	}

	// Verify if messageId read from the ack buffer is correct
	if messageIdFromAck != messageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", messageId, messageIdFromAck)
	}

	// Verify if the task added to the engine is correct
	expectedTask := &api.Task{
		Arn: taskArn,
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
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
