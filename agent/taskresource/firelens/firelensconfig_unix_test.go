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
	testFluentdOptions = map[string]string{
		"@type":               "kinesis_firehose",
		"region":              "us-west-2",
		"deliver_stream_name": "my-stream",
		"include-pattern":     "*failure*",
		"exclude-pattern":     "*success*",
	}

	testFluentbitOptions = map[string]string{
		"Name":                "kinesis_firehose",
		"region":              "us-west-2",
		"deliver_stream_name": "my-stream",
		"include-pattern":     "*failure*",
		"exclude-pattern":     "*success*",
	}

	expectedFluentdBridgeModeConfig = `
<source>
    @type unix
    path /var/run/fluent.sock
</source>

<source>
    @type forward
    bind 0.0.0.0
    port 24224
</source>

<filter container-firelens**>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens**>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

<filter **>
    @type record_transformer
    <record>
        ec2_instance_id i-123456789a
        ecs_cluster mycluster
        ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
        ecs_task_definition taskdefinition:1
    </record>
</filter>

@include /tmp/dummy.conf

<match container-firelens**>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`
	expectedFluentdAWSVPCConfig = `
<source>
    @type unix
    path /var/run/fluent.sock
</source>

<source>
    @type forward
    bind 127.0.0.1
    port 24224
</source>

<filter container-firelens**>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens**>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

<filter **>
    @type record_transformer
    <record>
        ec2_instance_id i-123456789a
        ecs_cluster mycluster
        ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
        ecs_task_definition taskdefinition:1
    </record>
</filter>

@include /tmp/dummy.conf

<match container-firelens**>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`
	expectedFluentdDefaultModeConfig = `
<source>
    @type unix
    path /var/run/fluent.sock
</source>

<filter container-firelens**>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens**>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

<filter **>
    @type record_transformer
    <record>
        ec2_instance_id i-123456789a
        ecs_cluster mycluster
        ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
        ecs_task_definition taskdefinition:1
    </record>
</filter>

@include /tmp/dummy.conf

<match container-firelens**>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`
	expectedFluentbitConfig = `
[INPUT]
    Name forward
    unix_path /var/run/fluent.sock

[INPUT]
    Name forward
    Listen 0.0.0.0
    Port 24224

[INPUT]
    Name tcp
    Tag firelens-healthcheck
    Listen 127.0.0.1
    Port 8877

[FILTER]
    Name   grep
    Match container-firelens*
    Regex  log *failure*

[FILTER]
    Name   grep
    Match container-firelens*
    Exclude log *success*

[FILTER]
    Name record_modifier
    Match *
    Record ec2_instance_id i-123456789a
    Record ecs_cluster mycluster
    Record ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
    Record ecs_task_definition taskdefinition:1

@INCLUDE /fluent-bit/etc/external.conf

[OUTPUT]
    Name null
    Match firelens-healthcheck

[OUTPUT]
    Name kinesis_firehose
    Match container-firelens*
    deliver_stream_name my-stream
    region us-west-2
`
	expectedFluentdConfigWithoutECSMetadata = `
<source>
    @type unix
    path /var/run/fluent.sock
</source>

<source>
    @type forward
    bind 0.0.0.0
    port 24224
</source>

<filter container-firelens**>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens**>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

@include /tmp/dummy.conf

<match container-firelens**>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`

	expectedFluentbitConfigWithoutOutputSection = `
[INPUT]
    Name forward
    unix_path /var/run/fluent.sock

[INPUT]
    Name forward
    Listen 0.0.0.0
    Port 24224

[INPUT]
    Name tcp
    Tag firelens-healthcheck
    Listen 127.0.0.1
    Port 8877

[FILTER]
    Name   grep
    Match container-firelens*
    Regex  log *failure*

[FILTER]
    Name   grep
    Match container-firelens*
    Exclude log *success*

[FILTER]
    Name record_modifier
    Match *
    Record ec2_instance_id i-123456789a
    Record ecs_cluster mycluster
    Record ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
    Record ecs_task_definition taskdefinition:1

@INCLUDE /fluent-bit/etc/external.conf

[OUTPUT]
    Name null
    Match firelens-healthcheck
`
)

func TestGenerateFluentdBridgeModeConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, testRegion, bridgeNetworkMode, testFirelensOptionsFile, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdBridgeModeConfig, configBytes.String())
}

func TestGenerateFluentdAWSVPCModeConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, testRegion, awsvpcNetworkMode, testFirelensOptionsFile, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdAWSVPCConfig, configBytes.String())
}

func TestGenerateFluentdDefaultModeConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, testRegion, "", testFirelensOptionsFile, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdDefaultModeConfig, configBytes.String())
}

func TestGenerateFluentbitConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentbitOptions,
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testFirelensOptionsS3, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentBitConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentbitConfig, configBytes.String())
}

func TestGenerateFluentdConfigMissingOutputName(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": {
			"key1": "value1",
		},
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, testRegion, bridgeNetworkMode, testFirelensOptionsFile, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	_, err = firelensResource.generateConfig()
	assert.Error(t, err)
}

func TestGenerateFLuentbitConfigMissingOutputName(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": {
			"key1": "value1",
		},
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testFirelensOptionsFile, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	_, err = firelensResource.generateConfig()
	assert.Error(t, err)
}

func TestGenerateConfigWithECSMetadataDisabled(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}
	testFirelensOptions := map[string]string{
		"enable-ecs-log-metadata": "false",
		"config-file-type":        "file",
		"config-file-value":       "/tmp/dummy.conf",
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, testRegion, bridgeNetworkMode, testFirelensOptions, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdConfigWithoutECSMetadata, configBytes.String())
}

func TestGenerateConfigWithoutOutputSection(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": {
			"include-pattern": "*failure*",
			"exclude-pattern": "*success*",
		},
	}

	firelensResource, err := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, testRegion, bridgeNetworkMode, testFirelensOptionsS3, containerToLogOptions,
		nil, testExecutionCredentialsID)
	require.NoError(t, err)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentBitConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentbitConfigWithoutOutputSection, configBytes.String())
}
