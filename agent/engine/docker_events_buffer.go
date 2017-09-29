// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	docker "github.com/fsouza/go-dockerclient"
	"sync"
)

// InfiniteBuffer defines an unlimited buffer, where it reads from
// input channel and write to output channel.
type InfiniteBuffer struct {
	events   []*docker.APIEvents
	empty    bool
	waitDone chan struct{}
	count    int
	lock     sync.RWMutex
}

// NewInfiniteBuffer returns an InfiniteBuffer object
func NewInfiniteBuffer() *InfiniteBuffer {
	return &InfiniteBuffer{
		waitDone: make(chan struct{}),
	}
}

// Serve starts reading from the input channel and writes to the buffer
func (buffer *InfiniteBuffer) Serve(events chan *docker.APIEvents) {
	for event := range events {
		go buffer.Write(event)
	}
}

// Write writes the event into the buffer
func (buffer *InfiniteBuffer) Write(event *docker.APIEvents) {
	buffer.lock.Lock()
	defer buffer.lock.Unlock()

	// TODO filter the event type
	buffer.events = append(buffer.events, event)
	if buffer.empty {
		buffer.empty = false
		buffer.waitDone <- struct{}{}
	}
}

// Consume reads the buffer and write to a listener channel
func (buffer *InfiniteBuffer) Consume(in chan<- *docker.APIEvents) {
	for {
		buffer.lock.Lock()

		if len(buffer.events) == 0 {
			buffer.empty = true
			buffer.lock.Unlock()
			// wait event
			<-buffer.waitDone
		} else {
			event := buffer.events[0]
			buffer.events = buffer.events[1:]
			buffer.lock.Unlock()

			in <- event
		}
	}
}
