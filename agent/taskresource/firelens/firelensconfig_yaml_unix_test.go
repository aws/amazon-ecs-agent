//go:build linux && unit
// +build linux,unit

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

package firelens

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFluentbitOptionsYAMLS3 = map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "s3",
		"config-file-value":       "arn:aws:s3:::bucket/fluent-bit-custom.yaml",
	}

	testFluentbitOptionsYAMLFile = map[string]string{
		"enable-ecs-log-metadata": "true",
		"config-file-type":        "file",
		"config-file-value":       "/fluent-bit/etc/custom.yaml",
	}

	expectedFluentbitYAMLConfigBridgeModeS3 = `
includes:
  - "/fluent-bit/etc/external.yaml"
pipeline:
  inputs:
    - name: "forward"
      Mem_Buf_Limit: "50MB"
      unix_path: "/var/run/fluent.sock"

    - name: "forward"
      Listen: "0.0.0.0"
      Port: "24224"

    - name: "tcp"
      tag: "firelens-healthcheck"
      Listen: "127.0.0.1"
      Port: "8877"

  filters:
    - name: grep
      match: "container-firelens*"
      regex: "log *failure*"

    - name: grep
      match: "container-firelens*"
      exclude: "log *success*"

    - name: record_modifier
      match: "*"
      record:
        - "ec2_instance_id i-123456789a"
        - "ecs_cluster mycluster"
        - "ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a"
        - "ecs_task_definition taskdefinition:1"

  outputs:
    - name: "null"
      match: "firelens-healthcheck"

    - name: "kinesis_firehose"
      match: "container-firelens*"
      deliver_stream_name: "my-stream"
      region: "us-west-2"

`

	expectedFluentbitYAMLConfigFile = `
includes:
  - "/fluent-bit/etc/custom.yaml"
pipeline:
  inputs:
    - name: "forward"
      Mem_Buf_Limit: "50MB"
      unix_path: "/var/run/fluent.sock"

    - name: "forward"
      Listen: "0.0.0.0"
      Port: "24224"

    - name: "tcp"
      tag: "firelens-healthcheck"
      Listen: "127.0.0.1"
      Port: "8877"

  filters:
    - name: grep
      match: "container-firelens*"
      regex: "log *failure*"

    - name: grep
      match: "container-firelens*"
      exclude: "log *success*"

    - name: record_modifier
      match: "*"
      record:
        - "ec2_instance_id i-123456789a"
        - "ecs_cluster mycluster"
        - "ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a"
        - "ecs_task_definition taskdefinition:1"

  outputs:
    - name: "null"
      match: "firelens-healthcheck"

    - name: "kinesis_firehose"
      match: "container-firelens*"
      deliver_stream_name: "my-stream"
      region: "us-west-2"

`
)

func TestGenerateFluentbitYAMLConfigBridgeMode(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentbitOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testUser, testFluentbitOptionsYAMLS3, containerToLogOptions,
		nil, testExecutionCredentialsID, testContainerMemoryLimit, testIPCompatibility)
	require.NoError(t, err)

	cfg, err := firelensResource.generateYAMLConfig()
	require.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = writeFluentBitYAMLConfig(configBytes, cfg)
	require.NoError(t, err)
	assert.Equal(t, expectedFluentbitYAMLConfigBridgeModeS3, configBytes.String())
}

func TestGenerateFluentbitYAMLConfigFileExternalConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentbitOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testUser, testFluentbitOptionsYAMLFile, containerToLogOptions,
		nil, testExecutionCredentialsID, testContainerMemoryLimit, testIPCompatibility)
	require.NoError(t, err)

	cfg, err := firelensResource.generateYAMLConfig()
	require.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = writeFluentBitYAMLConfig(configBytes, cfg)
	require.NoError(t, err)
	assert.Equal(t, expectedFluentbitYAMLConfigFile, configBytes.String())
}

func TestGenerateFluentbitYAMLConfigMissingOutputName(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": {
			"key1": "value1",
		},
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testUser, testFluentbitOptionsYAMLS3, containerToLogOptions,
		nil, testExecutionCredentialsID, testContainerMemoryLimit, testIPCompatibility)
	require.NoError(t, err)

	_, err = firelensResource.generateYAMLConfig()
	assert.Error(t, err)
}

func TestUsesYAMLFluentBitConfig(t *testing.T) {
	testCases := []struct {
		name               string
		firelensConfigType string
		externalConfigType string
		externalConfigValu string
		expected           bool
	}{
		{
			name:               "fluentbit with yaml s3 config",
			firelensConfigType: FirelensConfigTypeFluentbit,
			externalConfigType: ExternalConfigTypeS3,
			externalConfigValu: "arn:aws:s3:::bucket/custom.yaml",
			expected:           true,
		},
		{
			name:               "fluentbit with classic s3 config",
			firelensConfigType: FirelensConfigTypeFluentbit,
			externalConfigType: ExternalConfigTypeS3,
			externalConfigValu: "arn:aws:s3:::bucket/custom.conf",
			expected:           false,
		},
		{
			name:               "fluentbit without external config",
			firelensConfigType: FirelensConfigTypeFluentbit,
			expected:           false,
		},
		{
			name:               "fluentd with yaml config value is not supported",
			firelensConfigType: FirelensConfigTypeFluentd,
			externalConfigType: ExternalConfigTypeS3,
			externalConfigValu: "arn:aws:s3:::bucket/custom.yaml",
			expected:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			firelensResource := &FirelensResource{
				firelensConfigType:  tc.firelensConfigType,
				externalConfigType:  tc.externalConfigType,
				externalConfigValue: tc.externalConfigValu,
			}
			assert.Equal(t, tc.expected, firelensResource.usesYAMLFluentBitConfig())
		})
	}
}
