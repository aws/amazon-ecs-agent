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
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type APIType int32
type MetricsEngine struct {
	collection     bool
	cfg            *config.Config
	ctx            context.Context
	Registry       *prometheus.Registry
	managedMetrics map[APIType]MetricsClient
}

const (
	DockerAPI APIType = iota
	TaskEngine
	StateManager
	ECSClient
)

// Maintained list of APIs for which we collect metrics. MetricsClients will be
// initialized using Factory method when a MetricsEngine is created.
var (
	managedAPIs = map[APIType]string{
		DockerAPI:    "Docker_API",
		TaskEngine:   "Task_Engine",
		StateManager: "State_Manager",
		ECSClient:    "ECS_Client",
	}
	MetricsEngineGlobal *MetricsEngine = &MetricsEngine{
		collection: false,
	}
)

// Function called during Agent start up to expose metrics on a local endpoint
func PublishMetrics() {
	if MetricsEngineGlobal.collection {
		MetricsEngineGlobal.publishMetrics()
	}
}

// Initializes the Global MetricsEngine used throughout Agent
// Currently, we use the Prometheus Global Default Registerer, which also collecs
// basic Go application metrics that we use (like memory usage).
// In future cases, we can use a custom Prometheus Registry to group metrics.
// For unit testing purposes, we only focus on API calls and use our own Registry
func MustInit(cfg *config.Config, registry ...*prometheus.Registry) {
	if !cfg.PrometheusMetricsEnabled {
		return
	}
	var registryToUse *prometheus.Registry
	if len(registry) > 0 {
		registryToUse = registry[0]
	} else {
		registryToUse = prometheus.DefaultRegisterer.(*prometheus.Registry)
	}
	MetricsEngineGlobal = NewMetricsEngine(cfg, registryToUse)
	MetricsEngineGlobal.collection = true
}

// We create a MetricsClient for all managed APIs (APIs for which we will collect
// metrics)
func NewMetricsEngine(cfg *config.Config, registry *prometheus.Registry) *MetricsEngine {
	metricsEngine := &MetricsEngine{
		cfg:            cfg,
		Registry:       registry,
		managedMetrics: make(map[APIType]MetricsClient),
	}
	for managedAPI, _ := range managedAPIs {
		aClient := NewMetricsClient(managedAPI, metricsEngine.Registry)
		metricsEngine.managedMetrics[managedAPI] = aClient
	}
	return metricsEngine
}

// Wrapper function that allows APIs to call a single function
func (engine *MetricsEngine) RecordDockerMetric(callName string) func() {
	return engine.recordGenericMetric(DockerAPI, callName)
}

// Wrapper function that allows APIs to call a single function
func (engine *MetricsEngine) RecordTaskEngineMetric(callName string) func() {
	return engine.recordGenericMetric(TaskEngine, callName)
}

// Wrapper function that allows APIs to call a single function
func (engine *MetricsEngine) RecordStateManagerMetric(callName string) func() {
	return engine.recordGenericMetric(StateManager, callName)
}

// Wrapper function that allows APIs to call a single function
func (engine *MetricsEngine) RecordECSClientMetric(callName string) func() {
	return engine.recordGenericMetric(ECSClient, callName)
}

// Records a call's start and returns a function to be deferred.
// Wrapper functions will use this function for GenericMetricsClients.
// If Metrics collection is enabled from the cfg, we record a metric with callID
// as an empty string (signaling a call start), and then return a function to
// record a second metric with a non-empty callID.
// We use a channel holding 1 bool to ensure that the FireCallEnd is called AFTER
// the FireCallStart (because these are done in separate go routines)
// Recording a metric in an API needs only a wrapper function that supplies the
// APIType and called using the following format:
// defer metrics.MetricsEngineGlobal.RecordMetricWrapper(callName)()
func (engine *MetricsEngine) recordGenericMetric(apiType APIType, callName string) func() {
	callStarted := make(chan bool, 1)
	if engine == nil || !engine.collection {
		return func() {
		}
	}
	callID := engine.recordMetric(apiType, callName, "", callStarted)
	return func() {
		engine.recordMetric(apiType, callName, callID, callStarted)
	}
}

func (engine *MetricsEngine) recordMetric(apiType APIType, callName, callID string, callStarted chan bool) string {
	return engine.managedMetrics[apiType].RecordCall(callID, callName, time.Now(), callStarted)
}

// Function that exposes all Agent Metrics on a given port.
func (engine *MetricsEngine) publishMetrics() {
	go func() {
		// Because we are using the DefaultRegisterer in Prometheus, we can use
		// the promhttp.Handler() function. In future cases for custom registers,
		// we can use promhttp.HandlerFor(customRegisterer, promhttp.HandlerOpts{})
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(fmt.Sprintf(":%d", config.AgentPrometheusExpositionPort), nil)
		if err != nil {
			seelog.Errorf("Error publishing metrics: %s", err.Error())
		}
	}()
}
