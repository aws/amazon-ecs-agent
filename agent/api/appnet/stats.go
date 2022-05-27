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
	"io"

	prometheus "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// parseServiceConnectStats method parses stats in prometheus format and converts it to prometheus client model. RawStats looks like below
//
// # TYPE MetricFamily1 counter
// MetricFamily1{dimensionA=value1, dimensionB=value2} 1
//
// # TYPE MetricFamily3 gauge
// MetricFamily3{dimensionA=value1, dimensionB=value2} 3
//
// # TYPE MetricFamily2 histogram
// MetricFamily2{dimensionX=value1, dimensionY=value2, le=0.5} 1
// MetricFamily2{dimensionX=value1, dimensionY=value2, le=1} 2
// MetricFamily2{dimensionX=value1, dimensionY=value2, le=5} 3
func parseServiceConnectStats(rawStats io.Reader) (map[string]*prometheus.MetricFamily, error) {
	var parser expfmt.TextParser
	stats, err := parser.TextToMetricFamilies(rawStats)
	if err != nil {
		return nil, err
	}
	return stats, nil
}
