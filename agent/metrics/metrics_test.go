//go:build linux && unit
// +build linux,unit

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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

// Create default config for Metrics. PrometheusMetricsEnabled is set to false
// by default, so it must be turned on
func getTestConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.PrometheusMetricsEnabled = true
	return cfg
}

// Tests if MetricsEngineGlobal variable is initialized and if all managed
// MetricsClients are initialized
func TestMetricsEngineInit(t *testing.T) {
	defer func() {
		MetricsEngineGlobal = &MetricsEngine{
			collection: false,
		}
	}()
	cfg := getTestConfig()
	MustInit(&cfg, prometheus.NewRegistry())
	assert.NotNil(t, MetricsEngineGlobal)
	assert.Equal(t, len(MetricsEngineGlobal.managedMetrics), len(managedAPIs))
}

// Tests if a default config will start Prometheus metrics. Should be disabled
// by default.
func TestDisablePrometheusMetrics(t *testing.T) {
	defer func() {
		MetricsEngineGlobal = &MetricsEngine{
			collection: false,
		}
	}()
	cfg := getTestConfig()
	cfg.PrometheusMetricsEnabled = false
	MustInit(&cfg, prometheus.NewRegistry())
	PublishMetrics()
	assert.False(t, MetricsEngineGlobal.collection)
}

// Mimicks metric collection of Docker API calls through Go routines. The method
// call to record a metric is the same used by various clients throughout Agent.
// We sleep the go routine to simulate "work" being done.
// We can determine the expected values for the metrics and create a map of them,
// which will then be used to verify the accuracy of the metrics collected.
func TestMetricCollection(t *testing.T) {
	defer func() {
		MetricsEngineGlobal = &MetricsEngine{
			collection: false,
		}
	}()
	cfg := getTestConfig()
	MustInit(&cfg, prometheus.NewRegistry())
	MetricsEngineGlobal.collection = true

	var DockerMetricSleepTime1 time.Duration = 1 * time.Second
	var DockerMetricSleepTime2 time.Duration = 2 * time.Second

	var wg sync.WaitGroup
	wg.Add(20)

	// These Go routines simulate metrics collection
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer MetricsEngineGlobal.RecordDockerMetric("START")()
			time.Sleep(DockerMetricSleepTime1)
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer MetricsEngineGlobal.RecordDockerMetric("START")()
			time.Sleep(DockerMetricSleepTime2)
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer MetricsEngineGlobal.RecordDockerMetric("STOP")()
			time.Sleep(DockerMetricSleepTime1)
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer MetricsEngineGlobal.RecordDockerMetric("STOP")()
			time.Sleep(DockerMetricSleepTime2)
		}()
	}
	wg.Wait()
	time.Sleep(time.Second)

	// This will gather all collected metrics and store them in a MetricFamily list
	// All metric families can be printed by looping over this variable using
	// fmt.Println(proto.MarshalTextString(metricFamilies[n])) where n = index
	metricFamilies, err := MetricsEngineGlobal.Registry.Gather()
	assert.NoError(t, err)

	// Here we set up the expectations. These are known values which make verfication
	// easier.
	expected := make(metricMap)
	expected["AgentMetrics_DockerAPI_call_count"] = make(map[string][]interface{})
	expected["AgentMetrics_DockerAPI_call_count"]["CallSTART"] = []interface{}{
		"COUNTER",
		10.0,
	}
	expected["AgentMetrics_DockerAPI_call_count"]["CallSTOP"] = []interface{}{
		"COUNTER",
		10.0,
	}
	expected["AgentMetrics_DockerAPI_call_duration"] = make(map[string][]interface{})
	expected["AgentMetrics_DockerAPI_call_duration"]["CallSTART"] = []interface{}{
		"GUAGE",
		0.0,
	}
	expected["AgentMetrics_DockerAPI_call_duration"]["CallSTOP"] = []interface{}{
		"GUAGE",
		0.0,
	}
	expected["AgentMetrics_DockerAPI_duration_seconds"] = make(map[string][]interface{})
	expected["AgentMetrics_DockerAPI_duration_seconds"]["CallSTART"] = []interface{}{
		"SUMMARY",
		1.5,
	}
	expected["AgentMetrics_DockerAPI_duration_seconds"]["CallSTOP"] = []interface{}{
		"SUMMARY",
		1.5,
	}
	// We will do a simple tree search to verify all metrics in metricsFamilies
	// are as expected
	assert.True(t, verifyStats(metricFamilies, expected), "Metrics are not accurate")
}

// A type for storing a Tree-based map. We map the MetricName to a map of metrics
// under that name. This second map indexes by MetricLabelName+MetricLabelValue to
// a slice MetricType and MetricValue.
// MetricName:metricLabelName+metricLabelValue:[metricType, metricValue]
type metricMap map[string]map[string][]interface{}

// In order to verify the MetricFamily with the expected metric values, we do a simple
// tree search to verify that all stats in the MetricFamily coincide with the expected
// metric values.
// This method only verifes that all metrics in var metricsReceived are present in
// var expectedMetrics
func verifyStats(metricsReceived []*dto.MetricFamily, expectedMetrics metricMap) bool {
	var threshhold float64 = 0.1 // Maximum threshhold for two metrics being equal
	for _, metricFamily := range metricsReceived {
		if metricList, found := expectedMetrics[metricFamily.GetName()]; found {
			for _, metric := range metricFamily.GetMetric() {
				if aMetric, found := metricList[metric.GetLabel()[0].GetName()+metric.GetLabel()[0].GetValue()]; found {
					metricTypeExpected := string(aMetric[0].(string))
					metricValExpected := float64(aMetric[1].(float64))
					switch metricTypeExpected {
					case "GUAGE":
						continue
					case "COUNTER":
						if !compareDiff(metricValExpected, metric.GetCounter().GetValue(), threshhold) {
							fmt.Printf("Does not match SUMMARY. Expected: %f, Received: %f\n", metricValExpected, metric.GetCounter().GetValue())
							return false
						}
					case "SUMMARY":
						if !compareDiff(metricValExpected, metric.GetSummary().GetSampleSum()/float64(metric.GetSummary().GetSampleCount()), threshhold) {
							fmt.Printf("Does not match SUMMARY. Expected: %f, Received: %f\n", metricValExpected, metric.GetSummary().GetSampleSum()/float64(metric.GetSummary().GetSampleCount()))
							return false
						}
					default:
						fmt.Println("Metric Type not recognized")
						return false
					}
				} else {
					fmt.Println("MetricLabel Name and Value combo not found")
					return false
				}
			}
		} else {
			fmt.Println("MetricName not found")
			return false
		}
	}
	return true
}

// Helper function to determine if two values (a and b) are within a percentage of a
func compareDiff(a, b, deltaMin float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= (a * deltaMin)
}
