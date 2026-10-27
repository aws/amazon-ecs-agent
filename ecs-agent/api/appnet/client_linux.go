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
	"context"
	"net/http"
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/pkg/errors"
	prometheus "github.com/prometheus/client_model/go"
)

const (
	appnetRequestMaxAttempts = 3
)

var (
	// Injection point for UTs
	oneSecondBackoffNoJitter = retry.NewExponentialBackoff(time.Second, time.Second, 0, 1)
)

// GetStats invokes Appnet Agent's stats API to retrieve ServiceConnect stats in prometheus format. This function expects
// an Appnet-Agent-hosted HTTP server listening on the UDS path passed in config.
//
// Failed requests are retried. Appnet Agent advances its stats window on its own timer, not when the window is read,
// so a retry returns the same window.
func (cl *AppNetAgentClient) GetStats(adminSocketPath string, statsRequest string) (map[string]*prometheus.MetricFamily, error) {
	var stats map[string]*prometheus.MetricFamily
	err := retry.RetryNWithBackoff(oneSecondBackoffNoJitter, appnetRequestMaxAttempts, func() error {
		resp, err := cl.performAppnetRequest(http.MethodGet, adminSocketPath, statsRequest)
		if err != nil {
			logger.Warn("Error invoking Appnet's stats endpoint", logger.Fields{
				"adminSocketPath": adminSocketPath,
				field.Error:       err,
			})
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			logger.Warn("Non-OK response from Appnet's stats endpoint", logger.Fields{
				"adminSocketPath": adminSocketPath,
				"statusCode":      resp.StatusCode,
			})
			return apierrors.NewRetriableError(apierrors.NewRetriable(isRetriableStatusCode(resp.StatusCode)),
				errors.Errorf("received non-OK HTTP status %v from Service Connect stats endpoint", resp.StatusCode))
		}
		stats, err = parseServiceConnectStats(resp.Body)
		if err != nil {
			// The same response would fail to parse again.
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// isRetriableStatusCode reports whether a non-OK status from Appnet Agent may succeed on retry. 429 comes from Appnet
// Agent's rate limiter and 5xx from a failed Envoy stats scrape, and both can clear within seconds. Other 4xx statuses
// are caused by the request itself and do not change on retry.
func isRetriableStatusCode(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

// DrainInboundConnections invokes Appnet Agent's drain_listeners API which starts draining ServiceConnect inbound connections.
// This function expects an Appnet-agent-hosted HTTP server listening on the UDS path passed in config.
func (cl *AppNetAgentClient) DrainInboundConnections(adminSocketPath string, drainRequest string) error {
	return retry.RetryNWithBackoff(oneSecondBackoffNoJitter, 3, func() error {
		resp, err := cl.performAppnetRequest(http.MethodGet, adminSocketPath, drainRequest)
		if err != nil {
			logger.Warn("Error invoking Appnet's DrainInboundConnections", logger.Fields{
				"adminSocketPath": adminSocketPath,
				field.Error:       err,
			})
			return err
		}
		defer resp.Body.Close()
		return nil
	})
}

func (cl *AppNetAgentClient) performAppnetRequest(method, udsPath, url string) (*http.Response, error) {
	ctx := context.WithValue(context.Background(), udsAddressKey, udsPath)
	req, _ := http.NewRequestWithContext(ctx, method, url, nil)
	httpClient := cl.udsHttpClient
	return httpClient.Do(req)
}
