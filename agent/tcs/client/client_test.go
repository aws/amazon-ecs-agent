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

// Package tcsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by TCS. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.

package tcsclient

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/gorilla/websocket"
)

const (
	testPublishMetricsInterval = 1 * time.Second
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

func TestPayloadHandlerCalled(t *testing.T) {
	cs, ml := testCS()

	var handledPayload *ecstcs.AckPublishMetric
	reqHandler := func(payload *ecstcs.AckPublishMetric) {
		handledPayload = payload
	}
	cs.AddRequestHandler(reqHandler)

	ml.reads = [][]byte{[]byte(`{"type":"AckPublishMetric","message":{}}`)}

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

	isClosed = true
	cs.Close()
}

func TestPublishMetricsRequest(t *testing.T) {
	cs, _ := testCS()
	err := cs.MakeRequest(&ecstcs.PublishMetricsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	cs.Close()
}

func testCS() (wsclient.ClientServer, *messageLogger) {
	testCreds := auth.TestCredentialProvider{}
	cs := New("localhost:443", "us-east-1", testCreds, true, nil, testPublishMetricsInterval).(*clientServer)
	go cs.publishTimer.start(nil)
	ml := &messageLogger{make([][]byte, 0), make([][]byte, 0), false}
	cs.Conn = ml
	return cs, ml
}
