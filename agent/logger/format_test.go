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

package logger

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageTextFormatter_Format(t *testing.T) {
	f := messageTextFormatter{}
	result := f.Format("this is my message", Fields{
		"k1": "v1",
		"k2": 2,
		"k3": errors.New("the error"),
		"k4": struct {
			subK4_1 string
			subK4_2 int
		}{
			"subK4_1_Value",
			3,
		},
	})
	assert.True(t, strings.HasPrefix(result, `logger=structured `), `expected result to begin with: logger=structured `)
	assert.True(t, strings.Contains(result, `msg="this is my message" `), `expected result to start with: msg="this is my message"`)
	assert.True(t, strings.Contains(result, `k1="v1"`), `expected result to contain: k1="v1"`)
	assert.True(t, strings.Contains(result, `k2=2`), `expected result to contain: k2=2`)
	assert.True(t, strings.Contains(result, `k3="the error"`), `expected result to contain: k3="the error"`)
	assert.True(t, strings.Contains(result, `k4={subK4_1:subK4_1_Value subK4_2:3}`), `expected result to contain: k4={subK4_1:subK4_1_Value subK4_2:3}`)
}

func TestMessageJsonFormatter_Format(t *testing.T) {
	f := messageJsonFormatter{}
	result := f.Format("this is my message", Fields{
		"k1": "v1",
		"k2": 2,
		"k3": errors.New("the error"),
		"k4": struct {
			SubK4_1 string `json:"subK4_1"`
			SubK4_2 int    `json:"subK4_2"`
		}{
			"subK4_1_Value",
			3,
		},
	})
	assert.True(t, strings.HasPrefix(result, `"logger":"structured",`), `expected result to begin with:  "logger":"structured"`)
	require.JSONEq(t,
		`{"logger":"structured","k1":"v1","k2":2,"k3":"the error","k4":{"subK4_1":"subK4_1_Value","subK4_2":3},"msg":"this is my message"}`,
		"{"+result+"}")
}
