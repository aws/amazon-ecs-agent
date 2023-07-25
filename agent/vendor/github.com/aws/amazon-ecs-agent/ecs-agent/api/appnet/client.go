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
	"fmt"
	"net"
	"net/http"

	prometheus "github.com/prometheus/client_model/go"
)

type appnetClientCtxKey int

// AppnetClient is an interface with customized Appnet client that
// implements the GetStats and DrainInboundConnections
type AppNetClient interface {
	GetStats(adminSocketPath string, statsRequest string) (map[string]*prometheus.MetricFamily, error)
	DrainInboundConnections(adminSocketPath string, drainRequest string) error
}

type AppNetAgentClient struct {
	udsHttpClient http.Client
	AppNetClient
}

const (
	udsAddressKey   appnetClientCtxKey = iota
	unixNetworkName                    = "unix"
)

// Client retrieves the singleton Appnet client
func CreateClient() *AppNetAgentClient {
	client := AppNetAgentClient{
		udsHttpClient: http.Client{
			Transport: &http.Transport{
				DialContext: udsDialContext,
			},
		},
	}
	return &client
}

func udsDialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	udsAddress, ok := ctx.Value(udsAddressKey).(string)
	if !ok {
		return nil, fmt.Errorf("appnet client: Path to appnet admin socket was not a string")
	}
	if udsAddress == "" {
		return nil, fmt.Errorf("appnet client: Path to appnet admin socket was blank")
	}
	return net.Dial(unixNetworkName, udsAddress)
}
