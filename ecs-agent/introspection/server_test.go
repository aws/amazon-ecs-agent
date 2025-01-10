//go:build unit
// +build unit

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

package introspection

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var runtimeStatsConfigForTest = false

func TestNewServerErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	agentState := NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	t.Run("state is required", func(t *testing.T) {
		_, err := NewServer(nil, metricsFactory)
		assert.EqualError(t, err, "state cannot be nil")
	})
	t.Run("metrics factory is required", func(t *testing.T) {
		_, err := NewServer(agentState, nil)
		assert.EqualError(t, err, "metrics factory cannot be nil")
	})
}

func TestNewServerDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	agentState := NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	t.Run("read/write defaults", func(t *testing.T) {
		server, _ := NewServer(agentState, metricsFactory)
		assert.Equal(t, time.Duration(0), server.ReadTimeout)
		assert.Equal(t, time.Duration(0), server.WriteTimeout)
	})

	t.Run("profiling off by default", func(t *testing.T) {
		server, _ := NewServer(agentState, metricsFactory)

		req, err := http.NewRequest("GET", "/", nil)
		require.NoError(t, err)

		// Send the request and record the response
		recorder := httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, req)

		// Assert status code and body
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, `{"AvailableCommands":["/v1/metadata","/v1/tasks","/license"]}`, recorder.Body.String())
	})
}

func setupMockPprofHandlers() func() {
	runtimeStatsConfigForTestBkp := runtimeStatsConfigForTest
	pprofIndexHandlerBkp := pprofIndexHandler
	pprofCmdlineHandlerBkp := pprofCmdlineHandler
	pprofProfileHandlerBkp := pprofProfileHandler
	pprofSymbolHandlerBkp := pprofSymbolHandler
	pprofTraceHandlerBkp := pprofTraceHandler

	mockPprofTestHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Path))
	}
	pprofIndexHandler = mockPprofTestHandler
	pprofCmdlineHandler = mockPprofTestHandler
	pprofProfileHandler = mockPprofTestHandler
	pprofSymbolHandler = mockPprofTestHandler
	pprofTraceHandler = mockPprofTestHandler

	return func() {
		runtimeStatsConfigForTest = runtimeStatsConfigForTestBkp
		pprofIndexHandler = pprofIndexHandlerBkp
		pprofCmdlineHandler = pprofCmdlineHandlerBkp
		pprofProfileHandler = pprofProfileHandlerBkp
		pprofSymbolHandler = pprofSymbolHandlerBkp
		pprofTraceHandler = pprofTraceHandlerBkp
	}
}

func TestPProfHandlerSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	agentState := NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	pprofPaths := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
	}

	testCases := []struct {
		runtimeStatsEnabled bool
		paths               []string
	}{
		{runtimeStatsEnabled: false, paths: pprofPaths},
		{runtimeStatsEnabled: true, paths: pprofPaths},
	}

	for _, tc := range testCases {
		runtimeStatsConfigForTest = tc.runtimeStatsEnabled
		for _, p := range tc.paths {
			t.Run(p+"-"+strconv.FormatBool(runtimeStatsConfigForTest), func(t *testing.T) {
				defer setupMockPprofHandlers()
				server, _ := NewServer(agentState, metricsFactory, WithRuntimeStats(runtimeStatsConfigForTest))
				req, err := http.NewRequest("GET", p, nil)
				require.NoError(t, err)

				// Send the request and record the response
				recorder := httptest.NewRecorder()
				server.Handler.ServeHTTP(recorder, req)

				if runtimeStatsConfigForTest {
					assert.Equal(t, http.StatusOK, recorder.Code)
					assert.Equal(t, p, recorder.Body.String())
				} else {
					assert.Equal(t, http.StatusOK, recorder.Code)
					assert.Equal(t, `{"AvailableCommands":["/v1/metadata","/v1/tasks","/license"]}`, recorder.Body.String())
				}
			})
		}
	}
	for _, tc := range testCases {
		runtimeStatsConfigForTest = tc.runtimeStatsEnabled
		t.Run("default route - pprof: "+strconv.FormatBool(runtimeStatsConfigForTest), func(t *testing.T) {
			server, _ := NewServer(agentState, metricsFactory, WithRuntimeStats(runtimeStatsConfigForTest))
			req, err := http.NewRequest("GET", "/", nil)
			require.NoError(t, err)

			// Send the request and record the response
			recorder := httptest.NewRecorder()
			server.Handler.ServeHTTP(recorder, req)

			if runtimeStatsConfigForTest {
				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.Equal(t, `{"AvailableCommands":["/v1/metadata","/v1/tasks","/license",`+
					`"/debug/pprof/","/debug/pprof/cmdline","/debug/pprof/profile","/debug/pprof/symbol","/debug/pprof/trace"]}`, recorder.Body.String())
			} else {
				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.Equal(t, `{"AvailableCommands":["/v1/metadata","/v1/tasks","/license"]}`, recorder.Body.String())

			}
		})
	}
}
