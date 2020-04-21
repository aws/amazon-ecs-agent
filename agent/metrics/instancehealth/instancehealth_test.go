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
	wg.Add(10)

	//go routine simulates metric collection
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.IncrementCallCount()
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.RecordError(testError)
		}()
	}
	wg.Wait()

	expectedCallCount := int64(5)
	expectedErrorCount := int64(5)
	expectedErrorMessage := "TestNamedError: TestMetricsCollection"

	actualCallCount, actualErrorCount := testDockerMetric.GetAndResetCount()

	assert.Equal(t, expectedCallCount, actualCallCount)
	assert.Equal(t, expectedErrorCount, actualErrorCount)
	assert.Equal(t, expectedErrorMessage, testDockerMetric.GetErrorMessage())
}

func TestResetCount(t *testing.T) {
	testDockerMetric := &genericMetric{}
	testError := &testNamedError{errors.New("TestResetCount")}

	var wg sync.WaitGroup
	wg.Add(10)

	// go routine simulates metric collection
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.IncrementCallCount()
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer testDockerMetric.RecordError(testError)
		}()
	}
	wg.Wait()

	// Reset the counter
	testDockerMetric.GetAndResetCount()

	expectedDockerMetric := &genericMetric{}
	expectedDockerMetric.errorMessage.Store("TestNamedError: TestResetCount")

	assert.Equal(t, expectedDockerMetric, testDockerMetric, "Reset instance health counter failed")
}
