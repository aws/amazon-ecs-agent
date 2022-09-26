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
	"crypto/md5"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	callTimeout = 2 * time.Minute
)

// A GenericMetricsClient records 3 metrics:
//  1. A Prometheus summary vector representing call durations for different API calls
//  2. A durations guage vector that updates the last recorded duration for the API call
//     allowing for a time series view in the Prometheus browser
//  3. A counter vector that increments call counts for each API call
//
// The outstandingCalls map allows Fired CallStarts to be matched with Fired CallEnds
type GenericMetrics struct {
	durationVec      *prometheus.SummaryVec
	durations        *prometheus.GaugeVec
	counterVec       *prometheus.CounterVec
	lock             sync.RWMutex
	outstandingCalls map[string]time.Time
}

func Init() {
	rand.Seed(time.Now().UnixNano())
}

// This function creates a hash of the callName and a randomly generated number.
// The hash serves to match the call invokation with the call's end, which is
// required to compute the call duration. The final metric is observed when the
// FireCallEnd function is called.
// If callID is empty, then we initiate the call with FireCallStart
// We use a channel holding 1 bool to ensure that the FireCallEnd is called AFTER
// the FireCallStart (because these are done in separate go routines)
func (gm *GenericMetrics) RecordCall(callID, callName string, callTime time.Time, callStarted chan bool) string {
	if callID == "" {
		hashData := []byte("GenericMetrics-" + callName + strconv.FormatFloat(float64(rand.Float32()), 'f', -1, 32))
		hash := fmt.Sprintf("%x", md5.Sum(hashData))
		// Go routines are utilized to avoid blocking the main thread in case of
		// resource contention with var outstandingCalls
		go gm.FireCallStart(hash, callName, callTime, callStarted)
		go gm.IncrementCallCount(callName)
		return hash
	} else {
		go gm.FireCallEnd(callID, callName, callTime, callStarted)
		return ""
	}
}

func (gm *GenericMetrics) FireCallStart(callHash, callName string, timestamp time.Time, callStarted chan bool) {
	gm.lock.Lock()
	defer gm.lock.Unlock()
	// Check map to see if call is outstanding, otherwise, store in map
	if _, found := gm.outstandingCalls[callHash]; !found {
		gm.outstandingCalls[callHash] = timestamp
	} else {
		seelog.Errorf("Call is already outstanding: %s", callName)
	}
	callStarted <- true
}

func (gm *GenericMetrics) FireCallEnd(callHash, callName string, timestamp time.Time, callStarted chan bool) {
	defer func() {
		if r := recover(); r != nil {
			seelog.Errorf("FireCallEnd for %s panicked. Recovering quietly: %s", callName, r)
		}
	}()
	// We will block until the FireCallStart complement has completed
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	select {
	case <-ctx.Done():
		seelog.Errorf("FireCallEnd timed out with %s", callName)
		gm.lock.Lock()
		delete(gm.outstandingCalls, callHash)
		gm.lock.Unlock()
		return
	case <-callStarted:
		break
	}

	gm.lock.Lock()
	defer gm.lock.Unlock()
	// Check map to see if call is outstanding and calculate duration
	if timeStart, found := gm.outstandingCalls[callHash]; found {
		seconds := timestamp.Sub(timeStart)
		gm.durationVec.WithLabelValues(callName).Observe(seconds.Seconds())
		gm.durations.WithLabelValues(callName).Set(seconds.Seconds())
		delete(gm.outstandingCalls, callHash)
	} else {
		seelog.Errorf("Call is not outstanding: %s", callName)
	}
}

// This function increments the call count for a specific API call
// This is invoked at the API call's start, whereas the duration metrics
// are updated at the API call's end.
func (gm *GenericMetrics) IncrementCallCount(callName string) {
	defer func() {
		if r := recover(); r != nil {
			seelog.Errorf("IncrementCallCount for %s panicked. Recovering quietly: %s", callName, r)
		}
	}()
	gm.lock.Lock()
	defer gm.lock.Unlock()
	gm.counterVec.WithLabelValues(callName).Inc()
}
