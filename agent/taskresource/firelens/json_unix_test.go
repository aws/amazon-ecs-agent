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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

func TestMarshalUnmarshalJSON(t *testing.T) {
	testCreatedAt := time.Now()
	testContainerToLogOptions := map[string]map[string]string{
		"container": testFluentdOptions,
	}

	firelensResIn := &FirelensResource{
		cluster:               testCluster,
		taskARN:               testTaskARN,
		taskDefinition:        testTaskDefinition,
		ec2InstanceID:         testEC2InstanceID,
		resourceDir:           testResourceDir,
		firelensConfigType:    FirelensConfigTypeFluentd,
		ecsMetadataEnabled:    true,
		containerToLogOptions: testContainerToLogOptions,
		terminalReason:        testTerminalResason,
		createdAtUnsafe:       testCreatedAt,
		desiredStatusUnsafe:   resourcestatus.ResourceCreated,
		knownStatusUnsafe:     resourcestatus.ResourceCreated,
		appliedStatusUnsafe:   resourcestatus.ResourceCreated,
	}

	bytes, err := json.Marshal(firelensResIn)
	require.NoError(t, err)

	firelensResOut := &FirelensResource{}
	err = json.Unmarshal(bytes, firelensResOut)
	require.NoError(t, err)
	assert.Equal(t, testCluster, firelensResOut.cluster)
	assert.Equal(t, testTaskARN, firelensResOut.taskARN)
	assert.Equal(t, testTaskDefinition, firelensResOut.taskDefinition)
	assert.True(t, firelensResOut.ecsMetadataEnabled)
	assert.Equal(t, testContainerToLogOptions, firelensResOut.containerToLogOptions)
	assert.Equal(t, testTerminalResason, firelensResOut.terminalReason)
	// Can't use assert.Equal for time here. See https://github.com/golang/go/issues/22957.
	assert.True(t, testCreatedAt.Equal(firelensResOut.createdAtUnsafe))
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResOut.desiredStatusUnsafe)
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResOut.knownStatusUnsafe)
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResOut.appliedStatusUnsafe)
	assert.Equal(t, testTerminalResason, firelensResOut.terminalReason)
}
