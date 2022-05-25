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

package appnetclient

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	prometheus "github.com/prometheus/client_model/go"
)

const (
	unixNetworkName   = "unix"
	httpRequestPrefix = "http://" + unixNetworkName
	statsUrl          = httpRequestPrefix + "/stats/prometheus?usedonly&filter=metrics_extension&delta"
	drainUrl          = httpRequestPrefix + "/drain_listeners?inboundonly"
)

type appnetClientCtxKey int

const (
	udsAddressKey appnetClientCtxKey = iota
)

var (
	// Injection point for UTs
	performAppnetRequest = doPerformAppnetRequest
)

func udsDialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	udsPath, ok := ctx.Value(udsAddressKey).(string)
	if !ok {
		return nil, fmt.Errorf("appnet client: Path to appnet admin socket was not a string")
	}
	if udsPath == "" {
		return nil, fmt.Errorf("appnet client: Path to appnet admin socket was blank")
	}
	return net.Dial(unixNetworkName, udsPath)
}

var udsHttpClient = http.Client{
	Transport: &http.Transport{
		DialContext: udsDialContext,
	},
}

// GetStats invokes Appnet Agent's stats API to retrieve ServiceConnect stats in prometheus format. This function expects
// an Appnet-Agent-hosted HTTP server listening on the UDS path passed in config.
func GetStats(config task.RuntimeConfig) (map[string]*prometheus.MetricFamily, error) {
	resp, err := performAppnetRequest(http.MethodGet, config.AdminSocketPath, statsUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseServiceConnectStats(resp.Body)
}

// DrainInboundConnections invokes Appnet Agent's drain_listeners API which starts draining ServiceConnect inbound connections.
// This function expects an Appnet-agent-hosted HTTP server listening on the UDS path passed in config.
func DrainInboundConnections(config task.RuntimeConfig) error {
	resp, err := performAppnetRequest(http.MethodGet, config.AdminSocketPath, drainUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return err
}

func doPerformAppnetRequest(method, udsPath, url string) (*http.Response, error) {
	ctx := context.WithValue(context.Background(), udsAddressKey, udsPath)
	req, _ := http.NewRequestWithContext(ctx, method, url, nil)
	return udsHttpClient.Do(req)
}
