//go:build !linux

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

	prometheus "github.com/prometheus/client_model/go"
)

func (cl *AppNetAgentClient) GetStats(adminSocketPath string, statsRequest string) (map[string]*prometheus.MetricFamily, error) {
	return nil, fmt.Errorf("appnet client: GetStats is not supported in this platform")
}

func (cl *AppNetAgentClient) DrainInboundConnections(adminSocketPath string, drainRequest string) error {
	return fmt.Errorf("appnet client: DrainInboundConnections is not supported in this platform")
}
