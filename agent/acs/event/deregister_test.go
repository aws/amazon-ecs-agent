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
package event

import (
	"testing"
	"time"

	"golang.org/x/net/context"
)

type eventListener struct {
	called bool
}

func (listener *eventListener) recordCall() error {
	listener.called = true
	return nil
}

// TestSubscribe tests the listener subscribed to the
// event stream will be notified
func TestSubscribe(t *testing.T) {
	listener := eventListener{called: false}

	events := NewACSDeregisterInstanceStream()
	events.Subscribe(listener.recordCall)
	ctx, cancel := context.WithCancel(context.Background())

	go events.StartListening(ctx)

	events.EventChannel() <- struct{}{}
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

	events := NewACSDeregisterInstanceStream()

	events.Subscribe(listener1.recordCall)
	events.Subscribe(listener2.recordCall)
	ctx, cancel := context.WithCancel(context.Background())

	go events.StartListening(ctx)

	events.EventChannel() <- struct{}{}
	time.Sleep(1 * time.Second)
	if !listener1.called {
		t.Error("Listener 1 was not invoked")
	}
	if !listener2.called {
		t.Error("Listener 2 was not invoked")
	}

	listener1.called = false
	events.Unsubscribe(listener1.recordCall)
	events.EventChannel() <- struct{}{}

	if listener1.called {
		t.Error("Unsubscribed handler shouldn't be called")
	}
	cancel()
}

// TestCancelEventStream tests the event stream can
// be closed by context
func TestCancelEventStream(t *testing.T) {
	events := NewACSDeregisterInstanceStream()
	listener := eventListener{called: false}

	events.Subscribe(listener.recordCall)
	ctx, cancel := context.WithCancel(context.Background())

	go events.StartListening(ctx)
	cancel()

	events.EventChannel() <- struct{}{}
	if listener.called {
		t.Error("Cancelled events context, handler should not be called")
	}
}
