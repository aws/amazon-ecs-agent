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
package v4

import (
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Populate a ULongStatsSet with dummy value
func getDummyULongStatsSet() *ecstcs.ULongStatsSet {
	dummy := aws.Int64(0)
	return &ecstcs.ULongStatsSet{
		Max:         dummy,
		OverflowMax: dummy,
		Min:         dummy,
		OverflowMin: dummy,
		SampleCount: dummy,
		Sum:         dummy,
		OverflowSum: dummy,
	}
}

const bytesUtilized = "bytesUtilized"

func TestEphemeralStorageMetrics(t *testing.T) {
	cases := []struct {
		Name     string
		StatsSet *ecstcs.ULongStatsSet
	}{
		{
			Name:     "happy case",
			StatsSet: getDummyULongStatsSet(),
		},
		{
			Name:     "nil ULongStatsSet values",
			StatsSet: getDummyULongStatsSet(),
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Construct EphemeralStorageMetrics
			metrics := ecstcs.EphemeralStorageMetrics{
				BytesUtilized: test.StatsSet,
			}
			// Marshal to JSON (bytes)
			bytes, err := jsonutil.BuildJSON(&metrics)
			require.NoError(t, err)

			// validate that the JSON contains our expected key
			assert.Contains(t, string(bytes), bytesUtilized)

			// validate no errors
			errors := metrics.Validate()
			require.NoError(t, errors)
		})
	}
}

// Only Max/Min/SampleCount/Sum are checked for nil values in validate()
func TestEphemeralStorageMetricsNilValues(t *testing.T) {
	cases := []struct {
		Name  string
		field string
	}{
		{
			Name:  "nil max",
			field: "Max",
		},
		{
			Name:  "nil Min",
			field: "Min",
		},
		{
			Name:  "nil SampleCount",
			field: "SampleCount",
		},
		{
			Name:  "nil Sum",
			field: "Sum",
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Construct StatsSet
			statsSet := getDummyULongStatsSet()
			// Set field nil via reflection
			structValue := reflect.Indirect(reflect.ValueOf(statsSet))
			fieldValue := structValue.FieldByName(test.field)
			fieldValue.Set(reflect.Zero(fieldValue.Type()))

			// Construct EphemeralStorageMetrics
			metrics := ecstcs.EphemeralStorageMetrics{
				BytesUtilized: statsSet,
			}

			// validate errors
			errors := metrics.Validate()
			require.Error(t, errors)
		})
	}
}

// func TestEphemeralStorageMetrics(t *testing.T) {
// 	t.Run("happy case", func(t *testing.T) {
// 		dummy := aws.Int64(0)

// 		stats := &ecstcs.ULongStatsSet{
// 			Max:         dummy,
// 			OverflowMax: dummy,
// 			Min:         dummy,
// 			OverflowMin: dummy,
// 			SampleCount: dummy,
// 			Sum:         dummy,
// 			OverflowSum: dummy,
// 		}
// 		metrics := ecstcs.EphemeralStorageMetrics{
// 			BytesUtilized: stats,
// 		}
// 		err := metrics.Validate()
// 		require.NoError(t, err)
// 	})

// 	t.Run("nil ULongStatsSet", func(t *testing.T) {
// 		metrics := ecstcs.EphemeralStorageMetrics{
// 			BytesUtilized: nil,
// 		}
// 		err := metrics.Validate()
// 		require.NoError(t, err)
// 	})

// 	t.Run("nil ULongStatsSet values", func(t *testing.T) {
// 		metrics := ecstcs.EphemeralStorageMetrics{
// 			BytesUtilized: nil,
// 		}
// 		err := metrics.Validate()
// 		require.NoError(t, err)
// 	})
// }
