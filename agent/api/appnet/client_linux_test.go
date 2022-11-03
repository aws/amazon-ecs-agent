//go:build linux
// +build linux

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

package appnet

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gorilla/mux"
	prometheus "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

const (
	testUDSPath  = "/tmp/appnet_admin.sock"
	testStatsUrl = "http://thingie/stats/are/cool?true&key=value"
	testDrainUrl = "http://widget/drain/connections?all_of_them&key=value"
)

var (
	typeCounter      = prometheus.MetricType_COUNTER
	typeHistogram    = prometheus.MetricType_HISTOGRAM
	mockedValidStats = map[string]*prometheus.MetricFamily{
		"MetricFamily1": {
			Name: aws.String("MetricFamily1"),
			Type: &typeCounter,
			Metric: []*prometheus.Metric{
				{
					Label: []*prometheus.LabelPair{
						{
							Name:  aws.String("dimensionA"),
							Value: aws.String("value1"),
						},
						{
							Name:  aws.String("dimensionB"),
							Value: aws.String("value2"),
						},
					},
					Counter: &prometheus.Counter{
						Value: aws.Float64(1),
					},
				},
			},
		},
		"MetricFamily2": {
			Name: aws.String("MetricFamily2"),
			Type: &typeHistogram,
			Metric: []*prometheus.Metric{
				{
					Label: []*prometheus.LabelPair{
						{
							Name:  aws.String("dimensionX"),
							Value: aws.String("value1"),
						},
						{
							Name:  aws.String("dimensionY"),
							Value: aws.String("value2"),
						},
					},
					Histogram: &prometheus.Histogram{
						Bucket: []*prometheus.Bucket{
							{
								CumulativeCount: aws.Uint64(1),
								UpperBound:      aws.Float64(0.5),
							},
						},
					},
				},
			},
		},
	}
)

func setupTestUdsServer(t *testing.T, rawResponse string) *httptest.Server {
	t.Helper()
	r := mux.NewRouter()
	r.HandleFunc("/stats/are/cool", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.True(t, r.URL.Query().Has("true"))
		assert.Equal(t, "value", r.URL.Query().Get("key"))
		fmt.Fprintf(w, "%s", rawResponse)
	})
	r.HandleFunc("/drain/connections", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.True(t, r.URL.Query().Has("all_of_them"))
		assert.Equal(t, "value", r.URL.Query().Get("key"))
		fmt.Fprintf(w, "%s", rawResponse)
	})
	ts := httptest.NewUnstartedServer(r)
	l, err := net.Listen("unix", testUDSPath)
	if err != nil {
		t.Fatal("Error setting up test UDS HTTP server", err)
	}
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	return ts
}

func TestGetStats(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		udsPath               string
		rawStats              string
		expectedResult        map[string]*prometheus.MetricFamily
		isErrorExpected       bool
		expectedErrorContains string
	}{
		{
			name:    "happy case with valid counter and histogram metrics",
			udsPath: testUDSPath,
			rawStats: `# TYPE MetricFamily1 counter
			MetricFamily1{dimensionA="value1", dimensionB="value2"} 1
			# TYPE MetricFamily2 histogram
			MetricFamily2{dimensionX="value1", dimensionY="value2", le="0.5"} 1
			`,
			expectedResult:  mockedValidStats,
			isErrorExpected: false,
		},
		{
			name:    "sad case with invalid metrics",
			udsPath: testUDSPath,
			rawStats: `# TYPE MetricFamily1 counter
			bad metric 1
			# TYPE MetricFamily2 histogram
			bad metric 2
			`,
			expectedResult:        nil,
			isErrorExpected:       true,
			expectedErrorContains: "text format parsing error",
		},
		{
			name:    "invalid UDS path",
			udsPath: "/this/doesnt/exist.sock",
			rawStats: `# TYPE MetricFamily1 counter
			MetricFamily1{dimensionA="value1", dimensionB="value2"} 1
			# TYPE MetricFamily2 histogram
			MetricFamily2{dimensionX="value1", dimensionY="value2", le="0.5"} 1
			`,
			expectedResult:        nil,
			isErrorExpected:       true,
			expectedErrorContains: "dial unix /this/doesnt/exist.sock: connect: no such file or directory",
		},
		{
			name:    "blank UDS path",
			udsPath: "",
			rawStats: `# TYPE MetricFamily1 counter
			MetricFamily1{dimensionA="value1", dimensionB="value2"} 1
			# TYPE MetricFamily2 histogram
			MetricFamily2{dimensionX="value1", dimensionY="value2", le="0.5"} 1
			`,
			expectedResult:        nil,
			isErrorExpected:       true,
			expectedErrorContains: "appnet client: Path to appnet admin socket was blank",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := setupTestUdsServer(t, tc.rawStats)
			t.Cleanup(func() {
				ts.Close()
			})
			stats, err := Client().GetStats(serviceconnect.RuntimeConfig{AdminSocketPath: tc.udsPath, StatsRequest: testStatsUrl, DrainRequest: testDrainUrl})
			assert.Equal(t, tc.expectedResult, stats)
			if tc.isErrorExpected {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}

}

func TestDrainInboundConnections(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		udsPath               string
		isErrorExpected       bool
		expectedErrorContains string
	}{
		{
			name:            "happy case with valid uds path",
			udsPath:         testUDSPath,
			isErrorExpected: false,
		},
		{
			name:                  "sad case with invalid uds path",
			udsPath:               "/this/doesnt/exist.sock",
			isErrorExpected:       true,
			expectedErrorContains: "dial unix /this/doesnt/exist.sock: connect: no such file or directory",
		},
		{
			name:                  "sad case with blank uds path",
			udsPath:               "",
			isErrorExpected:       true,
			expectedErrorContains: "appnet client: Path to appnet admin socket was blank",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := setupTestUdsServer(t, "not important")
			t.Cleanup(func() {
				ts.Close()
			})
			err := Client().DrainInboundConnections(serviceconnect.RuntimeConfig{AdminSocketPath: tc.udsPath, StatsRequest: testStatsUrl, DrainRequest: testDrainUrl})
			if tc.isErrorExpected {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
