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

//go:build unit

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeFailureTrackerMetricsFactory is a minimal EntryFactory that counts
// Done(non-nil) calls.
type fakeFailureTrackerMetricsFactory struct{ numEmissions int }

func (f *fakeFailureTrackerMetricsFactory) New(_ string) Entry {
	return &fakeFailureTrackerMetricsEntry{f: f}
}
func (f *fakeFailureTrackerMetricsFactory) Flush() {}

type fakeFailureTrackerMetricsEntry struct {
	f *fakeFailureTrackerMetricsFactory
}

func (e *fakeFailureTrackerMetricsEntry) WithFields(_ map[string]interface{}) Entry { return e }
func (e *fakeFailureTrackerMetricsEntry) WithCount(_ int) Entry                     { return e }
func (e *fakeFailureTrackerMetricsEntry) WithGauge(_ interface{}) Entry             { return e }
func (e *fakeFailureTrackerMetricsEntry) Done(err error) {
	if err != nil {
		e.f.numEmissions++
	}
}

func TestFailureTracker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		actions              func(ft *FailureTracker)
		expectedNumEmissions int
	}{
		{
			name: "no emission when not failing",
			actions: func(ft *FailureTracker) {
				ft.emitIfFailing()
			},
			expectedNumEmissions: 0,
		},
		{
			name: "no emission when failing for less than emitThreshold",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.emitIfFailing()
			},
			expectedNumEmissions: 0,
		},
		{
			name: "emits when failing for more than emitThreshold",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.failingSince = time.Now().Add(-2 * emitThreshold)
				ft.emitIfFailing()
			},
			expectedNumEmissions: 1,
		},
		{
			name: "emits on every call while stuck",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.failingSince = time.Now().Add(-2 * emitThreshold)
				ft.emitIfFailing()
				ft.emitIfFailing()
			},
			expectedNumEmissions: 2,
		},
		{
			name: "stops emitting after success",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.failingSince = time.Now().Add(-2 * emitThreshold)
				ft.emitIfFailing()
				ft.RecordSuccess()
				ft.emitIfFailing()
			},
			expectedNumEmissions: 1,
		},
		{
			name: "resets timer on new failure after success",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.failingSince = time.Now().Add(-2 * emitThreshold)
				ft.emitIfFailing()
				ft.RecordSuccess()
				ft.RecordFailure()
				// New streak just started, so less than emitThreshold elapsed.
				ft.emitIfFailing()
			},
			expectedNumEmissions: 1,
		},
		{
			name: "repeated RecordFailure does not reset timer",
			actions: func(ft *FailureTracker) {
				ft.RecordFailure()
				ft.failingSince = time.Now().Add(-2 * emitThreshold)
				ft.RecordFailure()
				ft.emitIfFailing()
			},
			expectedNumEmissions: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMetricsFactory := &fakeFailureTrackerMetricsFactory{}
			testFailureTracker := NewFailureTracker("TestFailureTrackerMetric", fakeMetricsFactory)
			tc.actions(testFailureTracker)
			require.Equal(t, tc.expectedNumEmissions, fakeMetricsFactory.numEmissions)
		})
	}
}
