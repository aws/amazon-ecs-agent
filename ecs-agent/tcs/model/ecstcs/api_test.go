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
package ecstcs

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Populate a ULongStatsSet with dummy value
func getDummyULongStatsSet() *ULongStatsSet {
	dummy := aws.Int64(0)
	return &ULongStatsSet{
		Max:         dummy,
		OverflowMax: dummy,
		Min:         dummy,
		OverflowMin: dummy,
		SampleCount: dummy,
		Sum:         dummy,
		OverflowSum: dummy,
	}
}

const bytesUtilizedField = "bytesUtilized"
const maxField = "max"
const minField = "min"
const overflowMaxField = "overflowMax"
const overflowMinField = "overflowMin"
const overflowSumField = "overflowSum"
const sampleCountField = "sampleCount"
const sumField = "sum"

/*
Tests cases of ULongStatsSet that do not raise an error during validate():
 1. Non-nil StatsSet
 3. Non-nil StatsSet with "some" nil values
    a. Nil OverflowMax
    b. Nil OverflowMin
    c. Nil OverflowSum
*/
func TestULongStatsSet(t *testing.T) {
	cases := []struct {
		Name     string
		StatsSet *ULongStatsSet
	}{
		{
			Name:     "happy case",
			StatsSet: getDummyULongStatsSet(),
		},
		{
			Name: "nil OverflowMax",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.OverflowMax = nil
				return s
			}(),
		},
		{
			Name: "nil OverflowMin",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.OverflowMin = nil
				return s
			}(),
		},
		{
			Name: "nil OverflowSum",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.OverflowSum = nil
				return s
			}(),
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Marshal to JSON (bytes)
			bytes, err := json.Marshal(test.StatsSet)
			require.NoError(t, err)

			// convert bytes to JSON (string)
			jsonString := string(bytes)

			// test required keys are present
			assert.Contains(t, jsonString, maxField)
			assert.Contains(t, jsonString, minField)
			assert.Contains(t, jsonString, sampleCountField)
			assert.Contains(t, jsonString, sumField)
			// test optional keys if non-nil
			if test.StatsSet.OverflowMax != nil {
				assert.Contains(t, jsonString, overflowMaxField)
			}
			if test.StatsSet.OverflowMin != nil {
				assert.Contains(t, jsonString, overflowMinField)
			}
			if test.StatsSet.OverflowSum != nil {
				assert.Contains(t, jsonString, overflowSumField)
			}

			// validate no errors
			errors := test.StatsSet.Validate()
			require.NoError(t, errors)
		})
	}
}

/*
Tests cases of ULongStatsSet that do raise an error during validate():
 1. Non-nil StatsSet with "some" nil values
    a. Nil Max
    b. Nil Min
    c. Nil SampleCount
    d. Nil Sum
*/
func TestULongStatsSetNilValues(t *testing.T) {
	cases := []struct {
		Name     string
		Field    string
		StatsSet *ULongStatsSet
	}{
		{
			Name:  "nil Max",
			Field: "Max",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Max = nil
				return s
			}(),
		},
		{
			Name:  "nil Min",
			Field: "Min",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Min = nil
				return s
			}(),
		},
		{
			Name:  "nil SampleCount",
			Field: "SampleCount",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.SampleCount = nil
				return s
			}(),
		},
		{
			Name:  "nil Sum",
			Field: "Sum",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Sum = nil
				return s
			}(),
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Build error object for comparison
			invalidParams := request.ErrInvalidParams{Context: "ULongStatsSet"}
			invalidParams.Add(request.NewErrParamRequired(test.Field))

			// validate specific error
			errors := test.StatsSet.Validate()
			assert.Equal(t, invalidParams, errors)
		})
	}
}

/*
Tests cases of EphemeralStorageMetrics that do not raise an error during validate():
 1. Non-nil BytesUtilized
 2. Nil BytesUtilized
*/
func TestEphemeralStorageMetrics(t *testing.T) {
	cases := []struct {
		Name     string
		StatsSet *ULongStatsSet
	}{
		{
			Name:     "happy case",
			StatsSet: getDummyULongStatsSet(),
		},
		{
			Name:     "nil StatsSet",
			StatsSet: nil,
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Construct EphemeralStorageMetrics
			metrics := EphemeralStorageMetrics{
				BytesUtilized: test.StatsSet,
			}
			// Marshal to JSON (bytes)
			bytes, err := json.Marshal(&metrics)
			require.NoError(t, err)

			// convert bytes to JSON (string)
			jsonString := string(bytes)

			// test optional keys if non-nil
			if test.StatsSet != nil {
				assert.Contains(t, jsonString, bytesUtilizedField)
			}

			// validate no errors
			errors := metrics.Validate()
			require.NoError(t, errors)
		})
	}
}

/*
Tests cases of EphemeralStorageMetrics that do raise an error during validate():
 1. Non-nil StatsSet with "some" nil values
    a. Nil Max
    b. Nil Min
    c. Nil SampleCount
    d. Nil Sum
*/
func TestEphemeralStorageMetricsNilValues(t *testing.T) {
	cases := []struct {
		Name     string
		StatsSet *ULongStatsSet
	}{
		{
			Name: "nil Max",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Max = nil
				return s
			}(),
		},
		{
			Name: "nil Min",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Min = nil
				return s
			}(),
		},
		{
			Name: "nil SampleCount",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.SampleCount = nil
				return s
			}(),
		},
		{
			Name: "nil Sum",
			StatsSet: func() *ULongStatsSet {
				s := getDummyULongStatsSet()
				s.Sum = nil
				return s
			}(),
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// Construct EphemeralStorageMetrics
			metrics := EphemeralStorageMetrics{
				BytesUtilized: test.StatsSet,
			}
			// Build error object for comparison
			err := test.StatsSet.Validate()
			invalidParams := request.ErrInvalidParams{Context: "EphemeralStorageMetrics"}
			invalidParams.AddNested("BytesUtilized", err.(request.ErrInvalidParams))

			// validate specific error
			errors := metrics.Validate()
			assert.Equal(t, invalidParams, errors)
		})
	}
}
