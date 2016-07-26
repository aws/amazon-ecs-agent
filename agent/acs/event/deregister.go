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

// Package handler deals with appropriately reacting to all ACS messages as well
// as maintaining the connection to ACS.
package event

import (
	"reflect"
	"sync"

	"github.com/cihub/seelog"
	"golang.org/x/net/context"
)

type eventHandler func() error

// ACSDeregisterInstanceStream is the event stream for deregistering container instance event
// that was received from ACS.
type ACSDeregisterInstanceStream struct {
	event        chan struct{}
	handlers     []eventHandler
	handlersLock sync.RWMutex
}

func NewACSDeregisterInstanceStream() *ACSDeregisterInstanceStream {
	return &ACSDeregisterInstanceStream{
		event: make(chan struct{}, 1),
	}
}

// Subscribe adds the handler to be called into ACSDeregisterInstanceStream
func (events *ACSDeregisterInstanceStream) Subscribe(handler eventHandler) {
	events.handlersLock.Lock()
	defer events.handlersLock.Unlock()

	events.handlers = append(events.handlers, handler)
}

// broadcast calls all handler's handler function
func (events *ACSDeregisterInstanceStream) broadcast() {
	events.handlersLock.RLock()
	defer events.handlersLock.RUnlock()

	seelog.Debug("Received deregister container instance event, invoking the handler")
	for _, handlerFunc := range events.handlers {
		go handlerFunc()
	}
}

// Unsubscribe deletes the handler from the ACSDeregisterInstanceStream
func (events *ACSDeregisterInstanceStream) Unsubscribe(handlerToUnsubscribe eventHandler) {
	events.handlersLock.Lock()
	defer events.handlersLock.Unlock()

	for i, handler := range events.handlers {
		if reflect.ValueOf(handler).Pointer() == reflect.ValueOf(handlerToUnsubscribe).Pointer() {
			events.handlers = append(events.handlers[:i], events.handlers[i+1:]...)
			return
		}
	}
}

// EventChannel returns the channel the event stream is listening
func (events *ACSDeregisterInstanceStream) EventChannel() chan struct{} {
	return events.event
}

// StartListening start listening to the event stream and handle received event
func (events *ACSDeregisterInstanceStream) StartListening(ctx context.Context) {
	go func() {
		seelog.Info("Start listening to deregister-container-instance event from ACS client")
		for {
			select {
			case <-events.event:
				events.broadcast()
			case <-ctx.Done():
				seelog.Info("Stopped listening to the deregister-container-instance events from ACS client")
				return
			}
		}
	}()
}
