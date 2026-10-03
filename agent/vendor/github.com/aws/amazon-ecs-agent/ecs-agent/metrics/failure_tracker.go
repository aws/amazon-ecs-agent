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

package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

const (
	// emitInterval is how much time the metric emission loop should wait
	// between each metric emission.
	emitInterval = 1 * time.Minute

	// emitThreshold is the duration of time that must elapse after failingSince
	// before metric can be emitted.
	emitThreshold = 1 * time.Minute
)

// FailureTracker tracks whether an operation has been in a failure state
// continuously for more than emitThreshold and, if so, emits a metric every
// emitInterval.
type FailureTracker struct {
	failingSince   time.Time
	isFailing      bool
	metricName     string
	metricsFactory EntryFactory
	mu             sync.Mutex
}

// NewFailureTracker creates a new FailureTracker.
func NewFailureTracker(metricName string, metricsFactory EntryFactory) *FailureTracker {
	return &FailureTracker{
		metricName:     metricName,
		metricsFactory: metricsFactory,
	}
}

// RecordSuccess marks a successful operation and resets the failure state.
func (ft *FailureTracker) RecordSuccess() {
	if ft == nil {
		return
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.isFailing = false
}

// RecordFailure marks the start of a failure period if not already failing.
func (ft *FailureTracker) RecordFailure() {
	if ft == nil {
		return
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if !ft.isFailing {
		ft.isFailing = true
		ft.failingSince = time.Now()
	}
}

// StartEmitLoop runs emitIfFailing once every emitInterval until ctx is cancelled.
// This function is meant to be called by the entity creating the FailureTracker itself.
func (ft *FailureTracker) StartEmitLoop(ctx context.Context) {
	if ft == nil {
		return
	}
	logger.Debug("FailureTracker emit loop started", logger.Fields{
		field.MetricName: ft.metricName,
	})
	ticker := time.NewTicker(emitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ft.emitIfFailing()
		case <-ctx.Done():
			logger.Debug("FailureTracker emit loop stopping (context closed)", logger.Fields{
				field.MetricName: ft.metricName,
			})
			return
		}
	}
}

// IsFailing returns whether the operation being tracked is currently failing.
func (ft *FailureTracker) IsFailing() bool {
	if ft == nil {
		return false
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.isFailing
}

// emitIfFailing emits metric if its associated operation has been
// continuously failing for an amount of time greater than emitThreshold.
func (ft *FailureTracker) emitIfFailing() {
	if ft == nil {
		return
	}
	ft.mu.Lock()
	isFailing := ft.isFailing
	failingSince := ft.failingSince
	ft.mu.Unlock()

	if !isFailing {
		return
	}
	if time.Since(failingSince) <= emitThreshold {
		return
	}
	ft.metricsFactory.New(ft.metricName).Done(
		fmt.Errorf("operation associated with metric %s"+
			" has been failing for more than %v", ft.metricName, emitThreshold),
	)
}
