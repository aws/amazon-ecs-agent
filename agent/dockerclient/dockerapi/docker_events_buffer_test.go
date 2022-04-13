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

package dockerapi

import (
	"context"
	"strconv"
	"testing"

	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
)

func TestProduceConsume(t *testing.T) {
	buffer := NewInfiniteBuffer()
	producer := make(chan events.Message)
	consumer := make(chan *events.Message)

	// Start the process of producer and consumer
	go buffer.StartListening(context.TODO(), producer)
	go buffer.Consume(consumer)

	// writing multiple events to the buffer
	go func() {
		for i := 0; i < 10000; i++ {
			producer <- events.Message{
				ID:     strconv.Itoa(i),
				Type:   containerTypeEvent,
				Status: "die",
			}
		}
	}()

	for i := 0; i < 10000; i++ {
		<-consumer
	}
}

func TestIgnoreEvents(t *testing.T) {
	buffer := NewInfiniteBuffer()
	producer := make(chan events.Message)
	consumer := make(chan *events.Message)

	// Start the process of producer and consumer
	go buffer.StartListening(context.TODO(), producer)
	go buffer.Consume(consumer)

	// event with empty ID
	producer <- events.Message{Type: containerTypeEvent, Status: "stop"}
	// event with wrong type
	producer <- events.Message{ID: "id", Status: "stop", Type: "image"}
	for _, event := range containerEvents {
		producer <- events.Message{ID: "id", Type: containerTypeEvent, Status: event + "invalid"}
	}

	buffer.lock.Lock()
	defer buffer.lock.Unlock()
	assert.Len(t, buffer.events, 0)
}
