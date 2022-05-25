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
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	prometheus "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

var (
	statsUrlFormat = "http://unix%v"
)

type ServiceConnectStats struct {
	stats []*ecstcs.GeneralMetricsWrapper
	sent  bool
	lock  sync.RWMutex
}

func newServiceConnectStats() (*ServiceConnectStats, error) {
	return &ServiceConnectStats{}, nil
}

// TODO [SC]: Add retries on failure to retrieve service connect stats
func (sc *ServiceConnectStats) retrieveServiceConnectStats(task *apitask.Task) {

	serviceConnectConfig := task.GetServiceConnectRuntimeConfig()

	// TODO [SC]: Use the formal appnet client to connect to the UDS and fetch stats
	httpclient := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				return net.Dial("unix", serviceConnectConfig.AdminSocketPath)
			},
		},
	}

	resp, err := httpclient.Get(fmt.Sprintf(statsUrlFormat, serviceConnectConfig.StatsRequest))
	if err != nil {
		logger.Error("Could not connect to service connect stats endpoint for task", logger.Fields{
			field.TaskID: task.GetID(),
			field.Error:  err,
		})
		return
	}

	statsCollectedList, err := parseStats(resp.Body, task.GetID())
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

func parseStats(rawStats io.Reader, taskId string) ([]*ecstcs.GeneralMetricsWrapper, error) {
	statsMap := make(map[string]*ecstcs.GeneralMetricsWrapper)
	mf, err := parseServiceConnectStats(rawStats)
	if err != nil {
		return nil, err
	}

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
			if metric.Label != nil {
				for _, d := range metric.Label {
					// TODO [SC]: New stats endpoint will surface a new dimension which indicates what is the MetricType.
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

/* This method parses stats in prometheus format and converts it to prometheus client model. RawStats looks like below
=====================================================
TYPE MetricFamily1 counter
MetricFamily1{dimensionA=value1, dimensionB=value2} 1

# TYPE MetricFamily3 gauge
MetricFamily3{dimensionA=value1, dimensionB=value2} 3

# TYPE MetricFamily2 histogram
MetricFamily2{dimensionX=value1, dimensionY=value2, le=0.5} 1
MetricFamily2{dimensionX=value1, dimensionY=value2, le=1} 2
MetricFamily2{dimensionX=value1, dimensionY=value2, le=5} 3
*/

func parseServiceConnectStats(rawStats io.Reader) (map[string]*prometheus.MetricFamily, error) {
	var parser expfmt.TextParser
	stats, err := parser.TextToMetricFamilies(rawStats)
	if err != nil {
		return nil, err
	}
	return stats, nil
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
