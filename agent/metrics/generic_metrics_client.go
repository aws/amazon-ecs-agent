// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"crypto/md5"
	"fmt"
	"github.com/cihub/seelog"
	"github.com/prometheus/client_golang/prometheus"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// A GenericMetricsClient records 3 metrics:
// 1) A Prometheus summary vector representing call durations for different API calls
// 2) A durations guage vector that updates the last recorded duration for the API call
// 	  allowing for a time series view in the Prometheus browser
// 3) A counter vector that increments call counts for each API call
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
func (dm *GenericMetrics) RecordCall(callID, callName string, callTime time.Time) string {
	if callID == "" {
		hashData := []byte("GenericMetrics-" + callName + strconv.FormatFloat(float64(rand.Float32()), 'f', -1, 32))
		hash := fmt.Sprintf("%x", md5.Sum(hashData))
		// Go routines are utilized to avoid blocking the main thread in case of
		// resource contention with var outstandingCalls
		go dm.FireCallStart(hash, callName, callTime)
		go dm.IncrementCallCount(callName)
		return hash
	} else {
		go dm.FireCallEnd(callID, callName, callTime)
		return ""
	}
}

func (dm *GenericMetrics) FireCallStart(callHash, callName string, timestamp time.Time) {
	dm.lock.Lock()
	defer dm.lock.Unlock()
	// Check map to see if call is outstanding, otherwise, store in map
	if _, found := dm.outstandingCalls[callHash]; !found {
		dm.outstandingCalls[callHash] = timestamp
	} else {
		seelog.Errorf("Call is already outstanding: %s", callName)
	}
}

func (dm *GenericMetrics) FireCallEnd(callHash, callName string, timestamp time.Time) {
	defer func() {
		if r := recover(); r != nil {
			seelog.Errorf("IncrementCallCount for %s panicked. Recovering quietly: %s", callName, r)
		}
	}()
	dm.lock.Lock()
	defer dm.lock.Unlock()
	// Check map to see if call is outstanding and calculate duration
	if timeStart, found := dm.outstandingCalls[callHash]; found {
		seconds := timestamp.Sub(timeStart)
		dm.durationVec.WithLabelValues(callName).Observe(seconds.Seconds())
		dm.durations.WithLabelValues(callName).Set(seconds.Seconds())
		delete(dm.outstandingCalls, callHash)
	} else {
		seelog.Error("Call is not outstanding: %s", callName)
	}
}

// This function increments the call count for a specific API call
// This is invoked at the API call's start, whereas the duration metrics
// are updated at the API call's end.
func (dm *GenericMetrics) IncrementCallCount(callName string) {
	defer func() {
		if r := recover(); r != nil {
			seelog.Errorf("IncrementCallCount for %s panicked. Recovering quietly: %s", callName, r)
		}
	}()
	dm.lock.Lock()
	defer dm.lock.Unlock()
	dm.counterVec.WithLabelValues(callName).Inc()
}
