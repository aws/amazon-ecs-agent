//go:build linux && unit

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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestRetrieveServiceConnectMetrics(t *testing.T) {
	t1 := &apitask.Task{
		Arn:               "t1",
		Family:            "f1",
		ENIs:              []*apieni.ENI{{ID: "ec2Id"}},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			{Name: "test"},
		},
		LocalIPAddressUnsafe: "127.0.0.1",
		ServiceConnectConfig: &apitask.ServiceConnectConfig{
			RuntimeConfig: apitask.RuntimeConfig{
				AdminSocketPath: "/tmp/appnet_admin.sock",
				StatsRequest:    "/stats/prometheus",
			},
		},
	}

	var tests = []struct {
		stats         string
		expectedStats []*ecstcs.GeneralMetricsWrapper
	}{
		{
			stats: `# TYPE MetricFamily1 counter
				MetricFamily1{dimensionA="value1", dimensionB="value2"} 1
				# TYPE MetricFamily2 counter
				MetricFamily2{dimensionB="value2", dimensionA="value1"} 1
				`,
			expectedStats: []*ecstcs.GeneralMetricsWrapper{
				{
					Dimensions: []*ecstcs.Dimension{
						{
							Key:   aws.String("dimensionA"),
							Value: aws.String("value1"),
						}, {
							Key:   aws.String("dimensionB"),
							Value: aws.String("value2"),
						}},
					GeneralMetrics: []*ecstcs.GeneralMetric{
						{
							MetricCounts: []*int64{aws.Int64(1)},
							MetricName:   aws.String("MetricFamily1"),
							MetricValues: []*float64{aws.Float64(1)},
						},
						{
							MetricCounts: []*int64{aws.Int64(1)},
							MetricName:   aws.String("MetricFamily2"),
							MetricValues: []*float64{aws.Float64(1)},
						},
					},
				},
			},
		},
		{
			stats: `# TYPE MetricFamily3 histogram
				MetricFamily3{dimensionX="value1", dimensionY="value2", le="0.5"} 1
				MetricFamily3{dimensionX="value1", dimensionY="value2", le="1"} 2
				MetricFamily3{dimensionX="value1", dimensionY="value2", le="5"} 3
				`,
			expectedStats: []*ecstcs.GeneralMetricsWrapper{
				{
					Dimensions: []*ecstcs.Dimension{
						{
							Key:   aws.String("dimensionX"),
							Value: aws.String("value1"),
						}, {
							Key:   aws.String("dimensionY"),
							Value: aws.String("value2"),
						}},
					GeneralMetrics: []*ecstcs.GeneralMetric{
						{
							MetricCounts: []*int64{aws.Int64(1), aws.Int64(1), aws.Int64(1)},
							MetricName:   aws.String("MetricFamily3"),
							MetricValues: []*float64{aws.Float64(0.5), aws.Float64(1), aws.Float64(5)},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		// Set up a mock http sever on the statsUrlpath
		mockUDSPath := "/tmp/appnet_admin.sock"
		r := mux.NewRouter()
		r.HandleFunc("/stats/prometheus", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%v", test.stats)
		}))

		ts := httptest.NewUnstartedServer(r)

		l, err := net.Listen("unix", mockUDSPath)
		assert.NoError(t, err)

		ts.Listener.Close()
		ts.Listener = l
		ts.Start()

		serviceConnectStats := &ServiceConnectStats{}
		serviceConnectStats.retrieveServiceConnectStats(t1)

		sortMetrics(serviceConnectStats.GetStats())
		sortMetrics(test.expectedStats)
		assert.Equal(t, test.expectedStats, serviceConnectStats.GetStats())
		ts.Close()
	}
}

func sortMetrics(metricList []*ecstcs.GeneralMetricsWrapper) {
	for _, metric := range metricList {
		sort.Slice(metric.GeneralMetrics, func(i, j int) bool {
			return *metric.GeneralMetrics[i].MetricName < *metric.GeneralMetrics[j].MetricName
		})
	}
}
