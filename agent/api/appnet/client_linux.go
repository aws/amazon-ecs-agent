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

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/pkg/errors"
	prometheus "github.com/prometheus/client_model/go"
)

var (
	// Injection point for UTs
	oneSecondBackoffNoJitter = retry.NewExponentialBackoff(time.Second, time.Second, 0, 1)
)

// GetStats invokes Appnet Agent's stats API to retrieve ServiceConnect stats in prometheus format. This function expects
// an Appnet-Agent-hosted HTTP server listening on the UDS path passed in config.
func (cl *client) GetStats(config serviceconnect.RuntimeConfig) (map[string]*prometheus.MetricFamily, error) {
	resp, err := cl.performAppnetRequest(http.MethodGet, config.AdminSocketPath, config.StatsRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Wrapf(err, "received non-OK HTTP status %v from Service Connect stats endpoint", resp.StatusCode)
	}
	return parseServiceConnectStats(resp.Body)
}

// DrainInboundConnections invokes Appnet Agent's drain_listeners API which starts draining ServiceConnect inbound connections.
// This function expects an Appnet-agent-hosted HTTP server listening on the UDS path passed in config.
func (cl *client) DrainInboundConnections(config serviceconnect.RuntimeConfig) error {
	return retry.RetryNWithBackoff(oneSecondBackoffNoJitter, 3, func() error {
		resp, err := cl.performAppnetRequest(http.MethodGet, config.AdminSocketPath, config.DrainRequest)
		if err != nil {
			logger.Warn("Error invoking Appnet's DrainInboundConnections", logger.Fields{
				"adminSocketPath": config.AdminSocketPath,
				field.Error:       err,
			})
			return err
		}
		defer resp.Body.Close()
		return nil
	})
}

func (cl *client) performAppnetRequest(method, udsPath, url string) (*http.Response, error) {
	ctx := context.WithValue(context.Background(), udsAddressKey, udsPath)
	req, _ := http.NewRequestWithContext(ctx, method, url, nil)
	httpClient := cl.udsHttpClient
	return httpClient.Do(req)
}
