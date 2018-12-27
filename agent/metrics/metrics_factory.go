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
	"time"

	"github.com/cihub/seelog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	AgentNamespace        = "AgentMetrics"
	DockerSubsystem       = "DockerAPI"
	TaskEngineSubsystem   = "TaskEngine"
	StateManagerSubsystem = "StateManager"
	ECSClientSubsystem    = "ECSClient"
)

// A factory method that enables various MetricsClients to be created.
func NewMetricsClient(api APIType, registry *prometheus.Registry) MetricsClient {
	switch api {
	case DockerAPI:
		return NewGenericMetricsClient(DockerSubsystem, registry)
	case TaskEngine:
		return NewGenericMetricsClient(TaskEngineSubsystem, registry)
	case StateManager:
		return NewGenericMetricsClient(StateManagerSubsystem, registry)
	case ECSClient:
		return NewGenericMetricsClient(ECSClientSubsystem, registry)
	default:
		seelog.Error("Unmanaged MetricsClient cannot be created.")
		return nil
	}
}

func NewGenericMetricsClient(subsystem string, registry *prometheus.Registry) *GenericMetrics {
	aDurationVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  AgentNamespace,
		Subsystem:  subsystem,
		Name:       "duration_seconds",
		Help:       subsystem + " call duration in seconds",
		Objectives: make(map[float64]float64),
	}, []string{"Call"})
	registry.MustRegister(aDurationVec)

	aCounterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: AgentNamespace,
		Subsystem: subsystem,
		Name:      "call_count",
	}, []string{"Call"})
	registry.MustRegister(aCounterVec)

	aGaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: AgentNamespace,
			Subsystem: subsystem,
			Name:      "call_duration",
			Help:      subsystem + " call duration in seconds individual",
		},
		[]string{"Call"})
	registry.MustRegister(aGaugeVec)

	genericMetrics := &GenericMetrics{
		durationVec:      aDurationVec,
		counterVec:       aCounterVec,
		durations:        aGaugeVec,
		outstandingCalls: make(map[string]time.Time),
	}
	return genericMetrics
}
