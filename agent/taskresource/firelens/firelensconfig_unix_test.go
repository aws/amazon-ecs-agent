// +build linux,unit

// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	expectedFluentdConfig = `
<source>
    @type unix
    path /var/run/fluent.sock
</source>

<filter container-firelens*>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens*>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

<filter *>
    @type record_transformer
    <record>
        ec2_instance_id i-123456789a
        ecs_cluster mycluster
        ecs_task_arn arn:aws:ecs:us-east-2:01234567891011:task/mycluster/3de392df-6bfa-470b-97ed-aa6f482cd7a
        ecs_task_definition taskdefinition:1
    </record>
</filter>

<match container-firelens*>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`
	expectedFluentbitConfig = `
[INPUT]
    Name forward
    unix_path /var/run/fluent.sock

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

<filter container-firelens*>
    @type  grep
    <regexp>
        key log
        pattern *failure*
    </regexp>
</filter>

<filter container-firelens*>
    @type  grep
    <exclude>
        key log
        pattern *success*
    </exclude>
</filter>

<match container-firelens*>
    @type kinesis_firehose
    deliver_stream_name my-stream
    region us-west-2
</match>
`
)

func TestGenerateFluentdConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResource := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, true, containerToLogOptions)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdConfig, configBytes.String())
}

func TestGenerateFluentbitConfig(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentbitOptions,
	}

	firelensResource := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, true, containerToLogOptions)

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

	firelensResource := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, true, containerToLogOptions)

	_, err := firelensResource.generateConfig()
	assert.Error(t, err)
}

func TestGenerateFLuentbitConfigMissingOutputName(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": {
			"key1": "value1",
		},
	}

	firelensResource := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentbit, true, containerToLogOptions)

	_, err := firelensResource.generateConfig()
	assert.Error(t, err)
}

func TestGenerateConfigWithECSMetadataDisabled(t *testing.T) {
	containerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResource := NewFirelensResource(testCluster, testTaskARN, testTaskDefinition, testEC2InstanceID,
		testDataDir, FirelensConfigTypeFluentd, false, containerToLogOptions)

	config, err := firelensResource.generateConfig()
	assert.NoError(t, err)

	configBytes := new(bytes.Buffer)
	err = config.WriteFluentdConfig(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, expectedFluentdConfigWithoutECSMetadata, configBytes.String())
}
