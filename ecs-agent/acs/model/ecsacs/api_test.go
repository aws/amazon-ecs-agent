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

package ecsacs

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustedExecutionConfigurationUnmarshal(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected TrustedExecutionConfiguration
	}{
		{
			name:  "all fields present",
			input: `{"isolationMode":"nitroEnclaves","attestationPolicy":"my-policy"}`,
			expected: TrustedExecutionConfiguration{
				IsolationMode:     aws.String("nitroEnclaves"),
				AttestationPolicy: aws.String("my-policy"),
			},
		},
		{
			name:  "only isolationMode",
			input: `{"isolationMode":"nitroEnclaves"}`,
			expected: TrustedExecutionConfiguration{
				IsolationMode: aws.String("nitroEnclaves"),
			},
		},
		{
			name:  "only attestationPolicy",
			input: `{"attestationPolicy":"some-policy"}`,
			expected: TrustedExecutionConfiguration{
				AttestationPolicy: aws.String("some-policy"),
			},
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: TrustedExecutionConfiguration{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actual TrustedExecutionConfiguration
			err := json.Unmarshal([]byte(tc.input), &actual)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestTrustedExecutionConfigurationMarshal(t *testing.T) {
	input := TrustedExecutionConfiguration{
		IsolationMode:     aws.String("nitroEnclaves"),
		AttestationPolicy: aws.String("my-policy"),
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var roundTripped TrustedExecutionConfiguration
	err = json.Unmarshal(data, &roundTripped)
	require.NoError(t, err)
	assert.Equal(t, input, roundTripped)
}

func TestTrustedExecutionConfigurationMarshalOmitsEmpty(t *testing.T) {
	input := TrustedExecutionConfiguration{
		IsolationMode: aws.String("nitroEnclaves"),
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "attestationPolicy")
}

func TestTrustedExecutionConfigurationInTask(t *testing.T) {
	taskJSON := `{
		"arn": "arn:aws:ecs:us-west-2:123456789012:task/my-task",
		"trustedExecutionConfiguration": {
			"isolationMode": "nitroEnclaves",
			"attestationPolicy": "my-attestation-policy"
		}
	}`

	var task Task
	err := json.Unmarshal([]byte(taskJSON), &task)
	require.NoError(t, err)
	require.NotNil(t, task.TrustedExecutionConfiguration)
	assert.Equal(t, "nitroEnclaves",
		aws.ToString(task.TrustedExecutionConfiguration.IsolationMode))
	assert.Equal(t, "my-attestation-policy",
		aws.ToString(task.TrustedExecutionConfiguration.AttestationPolicy))
}
