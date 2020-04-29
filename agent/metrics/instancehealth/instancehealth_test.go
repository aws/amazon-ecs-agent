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

package instancehealth

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// CannotInspectContainerError indicates any error when trying to inspect a container
type testNamedError struct {
	FromError error
}

func (err testNamedError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotInspectContainerError
func (err testNamedError) ErrorName() string {
	return "TestNamedError"
}

// TestMetricsCollection imitates collection of instance health metrics for
// Docker API calls and errors to verify the accuracy of metric collection
func TestMetricsCollection(t *testing.T) {
	testDockerMetric := &genericMetric{}
	testError := &testNamedError{errors.New("TestMetricsCollection")}

	var wg sync.WaitGroup
	wg.Add(int(MinimumSampleCount) * 3)

	//go routine simulates metric collection
	for i := 0; i < int(MinimumSampleCount*2); i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.IncrementCallCount()
		}()
	}
	for i := 0; i < int(MinimumSampleCount); i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.RecordError(testError)
		}()
	}
	wg.Wait()

	expectedCallCount := MinimumSampleCount * 2
	expectedErrorCount := MinimumSampleCount
	expectedErrorMessage := "TestNamedError: TestMetricsCollection -- "

	actualCallCount, actualErrorCount, ok := testDockerMetric.GetAndResetCount()
	assert.True(t, ok)
	assert.Equal(t, expectedCallCount, actualCallCount)
	assert.Equal(t, expectedErrorCount, actualErrorCount)
	assert.Equal(t, expectedErrorMessage, testDockerMetric.GetAndResetErrorMessage())
}

func TestMetricsCollection_InsufficientSampleCount(t *testing.T) {
	testDockerMetric := &genericMetric{}
	testError := &testNamedError{errors.New("TestMetricsCollection")}

	// only two errors are not enough samples
	testDockerMetric.IncrementCallCount()
	testDockerMetric.RecordError(testError)

	_, _, ok := testDockerMetric.GetAndResetCount()

	assert.False(t, ok)
}

func TestResetCount(t *testing.T) {
	testDockerMetric := &genericMetric{}
	testError := &testNamedError{errors.New("TestResetCount")}

	for i := 0; i < int(MinimumSampleCount); i++ {
		testDockerMetric.IncrementCallCount()
		testDockerMetric.RecordError(testError)
	}

	// Reset the counter
	callCount, errCount, ok := testDockerMetric.GetAndResetCount()
	assert.Equal(t, int64(MinimumSampleCount), callCount)
	assert.Equal(t, int64(MinimumSampleCount), errCount)
	assert.True(t, ok)
	assert.Equal(t, "TestNamedError: TestResetCount -- ", testDockerMetric.GetAndResetErrorMessage())
}

// error message should remove duplicate messages and accumulate non-duplicate messages
func TestErrorMessage(t *testing.T) {
	testDockerMetric := &genericMetric{}
	testErrorA := &testNamedError{errors.New("docker error for container A")}
	testErrorB := &testNamedError{errors.New("docker error for container B")}

	for i := 0; i < int(MinimumSampleCount); i++ {
		testDockerMetric.IncrementCallCount()
		testDockerMetric.RecordError(testErrorA)
	}
	for i := 0; i < 5; i++ {
		testDockerMetric.IncrementCallCount()
		testDockerMetric.RecordError(testErrorB)
	}

	// Reset the counter
	callCount, errCount, ok := testDockerMetric.GetAndResetCount()
	assert.Equal(t, int64(MinimumSampleCount+5), callCount)
	assert.Equal(t, int64(MinimumSampleCount+5), errCount)
	assert.True(t, ok)
	assert.Equal(t, "TestNamedError: docker error for container A -- TestNamedError: docker error for container B -- ", testDockerMetric.GetAndResetErrorMessage())
}
