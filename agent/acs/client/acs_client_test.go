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

package acsclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/gorilla/websocket"
)

type messageLogger struct {
	writes [][]byte
	reads  [][]byte
	closed bool
}

func (ml *messageLogger) WriteMessage(_ int, data []byte) error {
	if ml.closed {
		return errors.New("can't write to closed ws")
	}
	ml.writes = append(ml.writes, data)
	return nil
}

func (ml *messageLogger) Close() error {
	ml.closed = true
	return nil
}

func (ml *messageLogger) ReadMessage() (int, []byte, error) {
	for len(ml.reads) == 0 && !ml.closed {
		time.Sleep(1 * time.Millisecond)
	}
	if ml.closed {
		return 0, []byte{}, errors.New("can't read from a closed websocket")
	}
	read := ml.reads[len(ml.reads)-1]
	ml.reads = ml.reads[0 : len(ml.reads)-1]
	return websocket.TextMessage, read, nil
}

func testCS() (ClientServer, *messageLogger) {
	testCreds := auth.TestCredentialProvider{}
	cs := New("localhost:443", "us-east-1", testCreds, true).(*clientServer)
	ml := &messageLogger{make([][]byte, 0), make([][]byte, 0), false}
	cs.conn = ml
	return cs, ml
}

func TestMakeUnrecognizedRequest(t *testing.T) {
	cs, _ := testCS()
	// 'testing.T' should not be a known type ;)
	err := cs.MakeRequest(t)
	if _, ok := err.(*UnrecognizedACSRequestType); !ok {
		t.Fatal("Expected unrecognized request type")
	}
	_ = err.Error() // This is one of those times when 100% coverage is silly
	cs.Close()
}

func strptr(s string) *string {
	return &s
}

func TestWriteAckRequest(t *testing.T) {
	cs, ml := testCS()

	req := ecsacs.AckRequest{Cluster: strptr("default"), ContainerInstance: strptr("testCI"), MessageId: strptr("messageID")}
	err := cs.MakeRequest(&req)
	if err != nil {
		t.Fatal(err)
	}

	write := ml.writes[0]
	writtenReq := struct {
		Type    string
		Message ecsacs.AckRequest
	}{}
	err = json.Unmarshal(write, &writtenReq)
	if err != nil {
		t.Fatal("Unable to unmarshal written", err)
	}
	msg := writtenReq.Message
	if *msg.Cluster != "default" || *msg.ContainerInstance != "testCI" || *msg.MessageId != "messageID" {
		t.Error("Did not write what we expected")
	}
	cs.Close()
}

func TestPayloadHandlerCalled(t *testing.T) {
	cs, ml := testCS()

	var handledPayload *ecsacs.PayloadMessage
	reqHandler := func(payload *ecsacs.PayloadMessage) {
		handledPayload = payload
	}
	cs.AddRequestHandler(reqHandler)

	ml.reads = [][]byte{[]byte(`{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn"}]}}`)}

	var isClosed bool
	go func() {
		err := cs.Serve()
		if !isClosed && err != nil {
			t.Fatal("Premature end of serving", err)
		}
	}()

	time.Sleep(1 * time.Millisecond)
	if handledPayload == nil {
		t.Fatal("Handler was not called")
	}

	if len(handledPayload.Tasks) != 1 || *handledPayload.Tasks[0].Arn != "arn" {
		t.Error("Unmarshalled data did not contain expected values")
	}

	isClosed = true
	cs.Close()
}

func TestClosingConnection(t *testing.T) {
	cs, ml := testCS()
	closedChan := make(chan error)
	var expectedClosed bool
	go func() {
		err := cs.Serve()
		if !expectedClosed {
			t.Fatal("Serve ended early")
		}
		closedChan <- err
	}()

	expectedClosed = true
	ml.Close()
	err := <-closedChan
	if err == nil {
		t.Error("Closing was expected to result in an error")
	}

	req := ecsacs.AckRequest{Cluster: strptr("default"), ContainerInstance: strptr("testCI"), MessageId: strptr("messageID")}
	err = cs.MakeRequest(&req)
	if err == nil {
		t.Error("Cannot request over closed connection")
	}
}

const (
	TestClusterArn  = "arn:aws:ec2:123:container/cluster:123456"
	TestInstanceArn = "arn:aws:ec2:123:container/containerInstance/12345678"
)

func startMockAcsServer(t *testing.T, closeWS <-chan bool) (*httptest.Server, chan<- string, <-chan string, <-chan error, error) {
	serverChan := make(chan string)
	requestsChan := make(chan string)
	errChan := make(chan error)

	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		go func() {
			<-closeWS
			ws.Close()
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
		for str := range serverChan {
			err := ws.WriteMessage(websocket.TextMessage, []byte(str))
			if err != nil {
				errChan <- err
			}
		}
	})

	server := httptest.NewTLSServer(handler)
	return server, serverChan, requestsChan, errChan, nil
}

func TestConnect(t *testing.T) {
	closeWS := make(chan bool)
	server, serverChan, requestChan, serverErr, err := startMockAcsServer(t, closeWS)
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		t.Fatal(<-serverErr)
	}()

	cs := New(server.URL, "us-east-1", auth.TestCredentialProvider{}, true)
	// Wait for up to a second for the mock server to launch
	for i := 0; i < 100; i++ {
		err = cs.Connect()
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error)
	cs.AddRequestHandler(func(msg *ecsacs.PayloadMessage) {
		if *msg.MessageId != "messageId" || len(msg.Tasks) != 1 || *msg.Tasks[0].Arn != "arn1" {
			errs <- errors.New("incorrect payloadMessage arguments")
		} else {
			errs <- nil
		}
	})

	go func() {
		_ = cs.Serve()
	}()

	go func() {
		serverChan <- `{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn1","desiredStatus":"RUNNING","overrides":"{}","family":"test","version":"v1","containers":[{"name":"c1","image":"redis","command":["arg1","arg2"],"cpu":10,"memory":20,"links":["db"],"portMappings":[{"containerPort":22,"hostPort":22}],"essential":true,"entryPoint":["bash"],"environment":{"key":"val"},"overrides":"{}","desiredStatus":"RUNNING"}]}],"messageId":"messageId"}}` + "\n"
	}()
	// Error for handling a 'PayloadMessage' request
	err = <-errs
	if err != nil {
		t.Fatal(err)
	}

	mid := "messageId"
	cluster := TestClusterArn
	ci := TestInstanceArn
	go func() {
		cs.MakeRequest(&ecsacs.AckRequest{
			MessageId:         &mid,
			Cluster:           &cluster,
			ContainerInstance: &ci,
		})
	}()

	request := <-requestChan

	// A request should have a 'type' and a 'message'
	intermediate := struct {
		Type    string             `json:"type"`
		Message *ecsacs.AckRequest `json:"message"`
	}{}
	err = json.Unmarshal([]byte(request), &intermediate)
	if err != nil {
		t.Fatal(err)
	}
	if intermediate.Type != "AckRequest" || *intermediate.Message.MessageId != mid || *intermediate.Message.ContainerInstance != ci || *intermediate.Message.Cluster != cluster {
		t.Fatal("Unexpected request")
	}
	closeWS <- true
	close(serverChan)
}

func TestConnectClientError(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"InvalidClusterException":"Invalid cluster"}` + "\n"))
	}))
	defer testServer.Close()

	cs := New(testServer.URL, "us-east-1", auth.TestCredentialProvider{}, true)
	err := cs.Connect()
	if _, ok := err.(*acsError); !ok || err.Error() != "InvalidClusterException: Invalid cluster" {
		t.Error("Did not get correctly typed error: " + err.Error())
	}
}
