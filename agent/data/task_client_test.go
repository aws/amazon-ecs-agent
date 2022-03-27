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

package data

import (
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTaskArn = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
)

func TestManageTask(t *testing.T) {
	testClient := newTestClient(t)

	testTask := &apitask.Task{
		Arn: testTaskArn,
	}

	require.NoError(t, testClient.SaveTask(testTask))
	res, err := testClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, res, 1)

	require.NoError(t, testClient.DeleteTask("abc"))
	res, err = testClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestSaveTaskInvalidID(t *testing.T) {
	testClient := newTestClient(t)

	testTask := &apitask.Task{
		Arn: "invalid-arn",
	}
	assert.Error(t, testClient.SaveTask(testTask))
}
