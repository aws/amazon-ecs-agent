//go:build linux

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

package stats

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/appnet"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	prometheus "github.com/prometheus/client_model/go"
)

type ServiceConnectStats struct {
	stats        []*ecstcs.GeneralMetricsWrapper
	appnetClient api.AppnetClient
	sent         bool
	lock         sync.RWMutex
}

const (
	ingress = "1"
	egress  = "2"
)

var directionToMetricType = map[string]string{
	"ingress": ingress,
	"egress":  egress,
}

func newServiceConnectStats() (*ServiceConnectStats, error) {
	return &ServiceConnectStats{
		appnetClient: appnet.Client(),
	}, nil
}

// TODO [SC]: Add retries on failure to retrieve service connect stats
func (sc *ServiceConnectStats) retrieveServiceConnectStats(task *apitask.Task) {
	stats, err := sc.appnetClient.GetStats(task.GetServiceConnectRuntimeConfig())
	if err != nil {
		logger.Error("Error retrieving Service Connect stats for task", logger.Fields{
			field.TaskID: task.GetID(),
			field.Error:  err,
		})
		return
	}

	statsCollectedList, err := convertToTACSStats(stats, task.GetID())
	if err != nil {
		logger.Error("Error parsing service-connect stats", logger.Fields{
			field.TaskID: task.GetID(),
			field.Error:  err,
		})
		return
	}
	sc.resetStats()
	sc.setStats(statsCollectedList)
}

func convertToTACSStats(mf map[string]*prometheus.MetricFamily, taskId string) ([]*ecstcs.GeneralMetricsWrapper, error) {
	statsMap := make(map[string]*ecstcs.GeneralMetricsWrapper)

	for _, v := range mf {
		for _, metric := range v.Metric {
			var metricValues []*float64
			var metricCounts []*int64
			metricCountForCountersAndGauges := int64(1)
			// Get Metric values and counts
			switch v.GetType() {
			case prometheus.MetricType_COUNTER:
				metricValues = append(metricValues, metric.Counter.Value)
				// MetricCount for Counter will always be [1]
				metricCounts = append(metricCounts, &metricCountForCountersAndGauges)
			case prometheus.MetricType_GAUGE:
				metricValues = append(metricValues, metric.Gauge.Value)
				// MetricCount for Gauge will always be [1]
				metricCounts = append(metricCounts, &metricCountForCountersAndGauges)
			case prometheus.MetricType_HISTOGRAM:
				for _, bucket := range metric.Histogram.Bucket {
					// We do not want to add the metricValue if it is +Inf
					if math.IsInf(bucket.GetUpperBound(), 0) {
						continue
					}
					metricValues = append(metricValues, bucket.UpperBound)

					// Prometheus histogram CumulativeCount is type *uint64, TACS wants *int64.
					var metricCount int64
					if bucket.GetCumulativeCount() <= uint64(math.MaxInt64) {
						metricCount = int64(bucket.GetCumulativeCount())
					} else {
						metricCount = math.MaxInt64
						logger.Warn("Service Connect histogram metric is emitting a value larger than max int64", logger.Fields{
							field.TaskID:            taskId,
							"metric":                v.Type.String(),
							"bucketCumulativeCount": bucket.GetCumulativeCount(),
						})
					}
					metricCounts = append(metricCounts, &metricCount)
				}
				metricCounts = convertHistogramMetricCounts(metricCounts)
			default:
				logger.Warn("Service connect stats received invalid Metric type", logger.Fields{
					field.TaskID: taskId,
					"metric":     v.Type.String(),
				})
				continue
			}

			generalMetric := &ecstcs.GeneralMetric{}
			generalMetric.MetricName = v.Name
			generalMetric.MetricValues = metricValues
			generalMetric.MetricCounts = metricCounts

			// Get metric dimensions
			var dimensions []*ecstcs.Dimension
			var metricType string
			if metric.Label != nil {
				for _, d := range metric.Label {
					if *d.Name == "Direction" {
						metricType = directionToMetricType[*d.Value]
						continue
					}

					dimension := &ecstcs.Dimension{
						Key:   d.Name,
						Value: d.Value,
					}
					dimensions = append(dimensions, dimension)
				}
			}

			dimensionAsString := sortAndConvertDimensionsintoStrings(dimensions)

			if generalMetricsWrapper, ok := statsMap[dimensionAsString]; !ok {
				// Dimension does not exist in statsMap, add it to the statsMap
				generalMetricsList := []*ecstcs.GeneralMetric{generalMetric}
				generalMetricsWrapper = &ecstcs.GeneralMetricsWrapper{
					Dimensions:     dimensions,
					GeneralMetrics: generalMetricsList,
					MetricType:     &metricType,
				}
				statsMap[dimensionAsString] = generalMetricsWrapper
			} else {
				// Add this metric to the metric list for the already existing dimesion
				generalMetricsWrapper.GeneralMetrics = append(generalMetricsWrapper.GeneralMetrics, generalMetric)
			}
		}
	}

	statsCollectedList := []*ecstcs.GeneralMetricsWrapper{}
	for _, gm := range statsMap {
		statsCollectedList = append(statsCollectedList, gm)
	}

	return statsCollectedList, nil
}

func (sc *ServiceConnectStats) setStats(stats []*ecstcs.GeneralMetricsWrapper) {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.stats = stats
}

func (sc *ServiceConnectStats) GetStats() []*ecstcs.GeneralMetricsWrapper {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	return sc.stats
}

func (sc *ServiceConnectStats) SetStatsSent(sent bool) {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.sent = sent
}

func (sc *ServiceConnectStats) HasStatsBeenSent() bool {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	return sc.sent
}

func (sc *ServiceConnectStats) resetStats() {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.stats = nil
	sc.sent = false
}

// CloudWatch accepts the histogram buckets in a disjoint manner while the prometheus emits these values in a cumulative way.
// This method performs that conversion
func convertHistogramMetricCounts(metricCounts []*int64) []*int64 {
	prevCount := *metricCounts[0]
	for i := 1; i < len(metricCounts); i++ {
		prevCount, *metricCounts[i] = *metricCounts[i], *metricCounts[i]-prevCount
	}
	return metricCounts
}

// This method sorts the dimensions according to the keyName.
// This helps to check if a dimension set already has a set of general metrics associated with it.
// Sorting helps us to take order of the dimension list into consideration.
// It then converts dimensions into strings. This is because we cannot
// have a slice as keys in maps
func sortAndConvertDimensionsintoStrings(dimension []*ecstcs.Dimension) string {
	sort.Slice(dimension, func(i, j int) bool {
		return *dimension[i].Key < *dimension[j].Key
	})

	return convertDimensionsintoStrings(dimension)
}

func convertDimensionsintoStrings(dimension []*ecstcs.Dimension) string {
	var sb strings.Builder
	for _, d := range dimension {
		sb.WriteString(*d.Key)
		sb.WriteString(*d.Value)
	}
	return sb.String()
}
