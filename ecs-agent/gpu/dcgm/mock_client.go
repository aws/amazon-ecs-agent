//go:build unit && linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package dcgm

import (
	"context"
	"fmt"
	"sync"

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
)

// MockClient is a mock implementation of Client for testing.
type MockClient struct {
	// Mock state.
	initialized       bool
	healthy           bool
	connectionLost    bool
	unhealthyReason   string
	reconcileError    error
	shutdownErr       error
	reconcileCalled   int
	shutdownCalled    int
	reconcileJustInit bool

	// Mock metrics state.
	metrics         []gputypes.GPUMetric
	metricsError    error
	getMetricsCalls int

	// Mutex for thread-safe access.
	mu sync.RWMutex
}

// NewMockClient creates a new mock DCGM client for testing.
func NewMockClient() *MockClient {
	return &MockClient{
		initialized: false,
		healthy:     false,
	}
}

// SetHealthy sets the mock client's health status.
func (m *MockClient) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

// SetInitialized sets the mock client's initialized status.
func (m *MockClient) SetInitialized(initialized bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = initialized
}

// SetReconcileError sets the error to return from Reconcile.
func (m *MockClient) SetReconcileError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileError = err
}

// SetReconcileJustInit sets whether Reconcile should return justInit=true.
func (m *MockClient) SetReconcileJustInit(justInit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileJustInit = justInit
}

// SetShutdownError sets the error to return from Shutdown.
func (m *MockClient) SetShutdownError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownErr = err
}

// SetConnectionLost sets the mock client's connection lost status.
func (m *MockClient) SetConnectionLost(lost bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionLost = lost
}

// SetUnhealthyReason sets the mock client's unhealthy reason.
func (m *MockClient) SetUnhealthyReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unhealthyReason = reason
}

// UnhealthyReason implements Client.UnhealthyReason.
func (m *MockClient) UnhealthyReason() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.unhealthyReason
}

// GetReconcileCallCount returns the number of times Reconcile was called.
func (m *MockClient) GetReconcileCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reconcileCalled
}

// GetShutdownCallCount returns the number of times Shutdown was called.
func (m *MockClient) GetShutdownCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shutdownCalled
}

// Reconcile implements Client.Reconcile.
func (m *MockClient) Reconcile(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileCalled++
	if m.reconcileError != nil {
		return false, m.reconcileError
	}
	justInit := m.reconcileJustInit
	m.initialized = true
	return justInit, nil
}

// IsHealthy implements Client.IsHealthy.
func (m *MockClient) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return false
	}
	return m.healthy
}

// IsConnectionLost implements Client.IsConnectionLost.
func (m *MockClient) IsConnectionLost() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connectionLost
}

// Shutdown implements Client.Shutdown.
func (m *MockClient) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled++
	if m.shutdownErr != nil {
		return m.shutdownErr
	}
	m.initialized = false
	return nil
}

// GetMetrics implements Client.GetMetrics.
func (m *MockClient) GetMetrics(ctx context.Context) ([]gputypes.GPUMetric, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getMetricsCalls++
	if m.metricsError != nil {
		return nil, m.metricsError
	}
	if !m.initialized {
		return nil, fmt.Errorf("DCGM client is not connected")
	}
	return m.metrics, nil
}

// SetMetrics sets the mock metrics to return from GetMetrics.
func (m *MockClient) SetMetrics(metrics []gputypes.GPUMetric) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = metrics
}

// SetMetricsError sets the error to return from GetMetrics.
func (m *MockClient) SetMetricsError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsError = err
}

// GetMetricsCallCount returns the number of times GetMetrics was called.
func (m *MockClient) GetMetricsCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getMetricsCalls
}
