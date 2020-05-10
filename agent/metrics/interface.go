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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
)

// MetricsClient defines the behavior for any Client that uses the
// MetricsEngine_ interface to collect metrics for Prometheus.
// Clients should all have the same type of metrics collected
// (Durations, counts, etc.)
// In particular, the API calls we monitor in the Agent code (TaskEngine,
// Docker, ECSClient, etc.) utilize call durations and counts which are
// encapsulated in the GenericMetricsClient.
// This interface is extensible for future clients that require different
// collection behaviors.
type MetricsClient interface {
	// RecordCall is responsible for accepting a call ID, call Name,
	// and timestamp for the call.
	// The specific behavior of RecordCall is dependent on the type of client
	// that is used. It is the responsibility of the MetricsEngine to call the
	// appropriate RecordCall(...) method.
	// This method is the defining function for this interface.
	// We use a channel holding 1 bool to ensure that the FireCallEnd is called AFTER
	// the FireCallStart (because these are done in separate go routines)
	RecordCall(string, string, time.Time, chan bool) string

	// In order to record call duration, we must fire a method's start and end
	// at different times (once at the beginning and once at the end). Ideally,
	// RecordCall should handle this through an execution and returned function
	// to be deferred (see docker_client.go for usage examples)
	FireCallStart(string, string, time.Time, chan bool)
	FireCallEnd(string, string, time.Time, chan bool)

	// This function will increment the call count. The metric for call duration
	// should ideally have the accurate call count as well, but the Prometheus
	// summary metric is updated for count when the FireCallEnd(...) function is
	// called. When a FireCallStart(...) is called but its FireCallEnd(...) is not,
	// we will see a discrepancy between the Summary's count and this Gauge count
	IncrementCallCount(string)
}

// MetricsEngine_ is an interface that drives metric collection over
// all existing MetricsClients. The APIType parameter corresponds to
// the type of MetricsClient RecordCall(...) function that will be used.
// The MetricsEngine_ should be instantiated at Agent startup time with
// Agent configurations and context passed as parameters
type MetricsEngine_ interface {
	// This init function initializes the Global MetricsEngine variable that
	// can be accessed throughout the Agent codebase on packages that the
	// metrics package does not depend on (cyclic dependencies not allowed).
	MustInit(*config.Config)

	// This function calls a specific APIType's Client's RecordCall(...) method.
	// As discussed in the comments for MetricsClient, different Clients have
	// have different RecordCall(...) behaviors. Wrapper functions for this
	// function should be created like recordGenericMetric(...) in metrics_engine.go
	// to record 2 metrics (function invokation and deferred function end).
	recordMetric(APIType, string, string)

	// publishMetrics is used to listen on defined ports on localhost
	// for incoming requests to expose metrics. An externally run Prometheus
	// server can then 'scrape' the exposed metrics for collection and storage
	publishMetrics()
}
