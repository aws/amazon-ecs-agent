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

package modeltransformer

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	firstThresholdVersion  = "1.10.0"
	secondThresholdVersion = "1.20.0"
)

type Test_task_1_0_0 struct {
	TestFieldId          string
	TestFieldContainerId string
	TestFieldTaskVCpu    string
}

type Test_task_1_10_0 struct {
	TestFieldId           string
	TestFieldContainerIds []string // breaking change introduced in 1.10.0
	TestFieldTaskVCpu     string
}

type Test_task_1_20_0 struct {
	TestFieldId           string
	TestFieldContainerIds []string
	TestFieldTaskVCpu     int // breaking change introduced in 1.20.0
}

func testTransformationFunction1100(dataIn []byte) ([]byte, error) {
	oldModel := Test_task_1_0_0{}
	newModel := Test_task_1_10_0{}

	err := json.Unmarshal(dataIn, &oldModel)
	if err != nil {
		return nil, err
	}

	newModel.TestFieldId = oldModel.TestFieldId
	newModel.TestFieldContainerIds = []string{oldModel.TestFieldContainerId}
	newModel.TestFieldTaskVCpu = oldModel.TestFieldTaskVCpu
	dataOut, err := json.Marshal(&newModel)
	return dataOut, err
}

func testTransformationFunction1200(dataIn []byte) ([]byte, error) {
	oldModel := Test_task_1_10_0{}
	newModel := Test_task_1_20_0{}

	err := json.Unmarshal(dataIn, &oldModel)
	if err != nil {
		return nil, err
	}

	newModel.TestFieldId = oldModel.TestFieldId
	newModel.TestFieldContainerIds = oldModel.TestFieldContainerIds
	newModel.TestFieldTaskVCpu, _ = strconv.Atoi(oldModel.TestFieldTaskVCpu)
	dataOut, err := json.Marshal(&newModel)
	return dataOut, err
}

func testTransformationFunctionBuggy(dataIn []byte) ([]byte, error) {
	err := errors.New("error")
	return []byte{}, err
}

func TestAddProblematicTransformationFunctionsAndTransformTaskFailed(t *testing.T) {
	boltDbMetadataVersion := "1.9.0"
	dataIn, _ := json.Marshal(Test_task_1_0_0{
		TestFieldId:          "id",
		TestFieldContainerId: "cid",
		TestFieldTaskVCpu:    "1",
	})
	transformer := NewTransformer()
	transformer.AddTaskTransformationFunctions(firstThresholdVersion, testTransformationFunctionBuggy)
	_, err := transformer.TransformTask(boltDbMetadataVersion, dataIn)
	assert.Error(t, err, "Expecting error when error returned from transformationFunction")
}

func TestAddTransformationFunctionsAndTransformTaskFailedCorruptedData(t *testing.T) {
	boltDbMetadataVersion := "1.19.0"
	dataIn, _ := json.Marshal(Test_task_1_10_0{
		TestFieldId:           "id",
		TestFieldContainerIds: []string{"cid"},
		TestFieldTaskVCpu:     "1",
	})
	corruptedDataIn := dataIn[1 : len(dataIn)-1]
	transformer := NewTransformer()
	transformer.AddTaskTransformationFunctions(firstThresholdVersion, testTransformationFunction1100)
	transformer.AddTaskTransformationFunctions(secondThresholdVersion, testTransformationFunction1200)
	_, err := transformer.TransformTask(boltDbMetadataVersion, corruptedDataIn)
	assert.Error(t, err, "Expecting error with corrupted json data persisted.")
}

func TestAddTaskTransformationFunctionsAndTransformTask(t *testing.T) {
	testCases := []struct {
		name                  string
		boltDbMetadataVersion string
		dataIn                interface{}
	}{
		{
			name:                  "upgrade skip all transformations",
			boltDbMetadataVersion: "1.20.0",
			dataIn: Test_task_1_20_0{
				TestFieldId:           "id",
				TestFieldContainerIds: []string{"cid"},
				TestFieldTaskVCpu:     1,
			},
		},
		{
			name:                  "upgrade skip first transformation",
			boltDbMetadataVersion: "1.19.0",
			dataIn: Test_task_1_10_0{
				TestFieldId:           "id",
				TestFieldContainerIds: []string{"cid"},
				TestFieldTaskVCpu:     "1",
			},
		},
		{
			name:                  "upgrade skip first transformation test 2",
			boltDbMetadataVersion: "1.10.0",
			dataIn: Test_task_1_10_0{
				TestFieldId:           "id",
				TestFieldContainerIds: []string{"cid"},
				TestFieldTaskVCpu:     "1",
			},
		},
		{
			name:                  "upgrade go through all transformations",
			boltDbMetadataVersion: "1.9.0",
			dataIn: Test_task_1_0_0{
				TestFieldId:          "id",
				TestFieldContainerId: "cid",
				TestFieldTaskVCpu:    "1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedOut, _ := json.Marshal(&Test_task_1_20_0{
				TestFieldId:           "id",
				TestFieldContainerIds: []string{"cid"},
				TestFieldTaskVCpu:     1,
			})
			dataIn, _ := json.Marshal(&tc.dataIn)
			transformer := NewTransformer()
			transformer.AddTaskTransformationFunctions(firstThresholdVersion, testTransformationFunction1100)
			transformer.AddTaskTransformationFunctions(secondThresholdVersion, testTransformationFunction1200)
			dataOut, err := transformer.TransformTask(tc.boltDbMetadataVersion, dataIn)
			assert.NoError(t, err, "Expected no error from transform, but there is.")
			assert.Equal(t, expectedOut, dataOut)
		})
	}
}

func TestCheckIsUpgrade(t *testing.T) {
	testCases := []struct {
		name                  string
		runningAgentVersion   string
		persistedAgentVersion string
		expect                bool
	}{
		{
			name:                  "runningAgentVersion equals to persistedAgentVersion",
			runningAgentVersion:   "1.0.0",
			persistedAgentVersion: "1.0.0",
			expect:                false,
		},
		{
			name:                  "runningAgentVersion greater than persistedAgentVersion",
			runningAgentVersion:   "1.1.0",
			persistedAgentVersion: "1.0.0",
			expect:                true,
		},
		{
			name:                  "runningAgentVersion smaller than persistedAgentVersion",
			runningAgentVersion:   "1.0.0",
			persistedAgentVersion: "1.1.0",
			expect:                false,
		},
		{
			name:                  "running agent version is corrupted",
			runningAgentVersion:   "1.0",
			persistedAgentVersion: "1.1.0",
			expect:                false,
		},
		{
			name:                  "persisted agent version is corrupted",
			runningAgentVersion:   "1.0.0",
			persistedAgentVersion: "1.1.x",
			expect:                false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transformer := NewTransformer()
			assert.Equal(t, tc.expect, transformer.IsUpgrade(tc.runningAgentVersion, tc.persistedAgentVersion))
		})
	}
}
