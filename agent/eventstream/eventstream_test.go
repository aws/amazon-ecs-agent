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
package eventstream

import (
	"testing"
	"time"

	"golang.org/x/net/context"
)

type eventListener struct {
	called bool
}

func (listener *eventListener) recordCall(...interface{}) error {
	listener.called = true
	return nil
}

// TestSubscribe tests the listener subscribed to the
// event stream will be notified
func TestSubscribe(t *testing.T) {
	listener := eventListener{called: false}

	ctx, cancel := context.WithCancel(context.Background())
	eventStream := NewEventStream("TestSubscribe", ctx)
	eventStream.Subscribe("listener", listener.recordCall)

	eventStream.StartListening()

	err := eventStream.WriteToEventStream(struct{}{})
	if err != nil {
		t.Errorf("Write to event stream failed, err: %v", err)
	}

	time.Sleep(1 * time.Second)

	cancel()
	if !listener.called {
		t.Error("Listener was not invoked")
	}
}

// TestUnsubscribe tests the listener unsubscribed from the
// event steam will not be notified
func TestUnsubscribe(t *testing.T) {
	listener1 := eventListener{called: false}
	listener2 := eventListener{called: false}

	ctx, cancel := context.WithCancel(context.Background())
	eventStream := NewEventStream("TestUnsubscribe", ctx)

	eventStream.Subscribe("listener1", listener1.recordCall)
	eventStream.Subscribe("listener2", listener2.recordCall)

	eventStream.StartListening()

	err := eventStream.WriteToEventStream(struct{}{})
	if err != nil {
		t.Errorf("Write to event stream failed, err: %v", err)
	}

	time.Sleep(1 * time.Second)
	if !listener1.called {
		t.Error("Listener 1 was not invoked")
	}
	if !listener2.called {
		t.Error("Listener 2 was not invoked")
	}

	listener1.called = false
	listener2.called = false
	eventStream.Unsubscribe("listener1")

	err = eventStream.WriteToEventStream(struct{}{})
	if err != nil {
		t.Errorf("Write to event stream failed, err: %v", err)
	}

	// wait for the event stream to broadcast
	time.Sleep(1 * time.Second)

	if listener1.called {
		t.Error("Unsubscribed handler shouldn't be called")
	}

	if !listener2.called {
		t.Error("Listener 2 was not invoked without unsubscribing")
	}
	cancel()
}

// TestCancelEventStream tests the event stream can
// be closed by context
func TestCancelEventStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	eventStream := NewEventStream("TestCancelEventStream", ctx)

	listener := eventListener{called: false}

	eventStream.Subscribe("listener", listener.recordCall)

	eventStream.StartListening()
	cancel()

	// wait for the event stream to handle cancel
	time.Sleep(1 * time.Second)

	err := eventStream.WriteToEventStream(struct{}{})
	if err == nil {
		t.Error("Write to closed event stream should return an error")
	}
	if listener.called {
		t.Error("Cancelled events context, handler should not be called")
	}
}
