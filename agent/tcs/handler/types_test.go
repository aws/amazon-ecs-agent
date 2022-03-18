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

package tcshandler

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestIsMetricsDisabled(t *testing.T) {
	tcs := []struct {
		param       *TelemetrySessionParams
		result      bool
		err         error
		description string
	}{
		{
			param:       &TelemetrySessionParams{},
			result:      false,
			err:         errors.New("Config is empty in the tcs session parameter"),
			description: "Config not set should cause error",
		},
		{
			param:       &TelemetrySessionParams{Cfg: &config.Config{DisableMetrics: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}, DisableDockerHealthCheck: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}}},
			result:      false,
			err:         nil,
			description: "No metrics was disable should return false",
		},
		{
			param:       &TelemetrySessionParams{Cfg: &config.Config{DisableMetrics: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}, DisableDockerHealthCheck: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}}},
			result:      false,
			err:         nil,
			description: "Only health metrics was disable should return false",
		},
		{
			param:       &TelemetrySessionParams{Cfg: &config.Config{DisableMetrics: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}, DisableDockerHealthCheck: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}}},
			result:      false,
			err:         nil,
			description: "Only telemetry metrics was disable should return false",
		},
		{
			param:       &TelemetrySessionParams{Cfg: &config.Config{DisableMetrics: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}, DisableDockerHealthCheck: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}}},
			result:      true,
			err:         nil,
			description: "both telemetry and health metrics were disable should return true",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			ok, err := tc.param.isContainerHealthMetricsDisabled()
			if tc.err != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.result, ok)
		})
	}
}
