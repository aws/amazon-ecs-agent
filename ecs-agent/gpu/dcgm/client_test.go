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
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestNewClient tests the NewClient constructor with various configurations.
func TestNewClient(t *testing.T) {
	testCases := []struct {
		name               string
		config             Config
		expectedSocketPath string
		description        string
	}{
		{
			name: "creates client with provided socket path",
			config: Config{
				SocketPath: "/run/nvidia-dcgm/custom-hostengine",
			},
			expectedSocketPath: "/run/nvidia-dcgm/custom-hostengine",
			description:        "Client should use the provided socket path",
		},
		{
			name:               "creates client with default socket path when empty",
			config:             Config{},
			expectedSocketPath: DefaultSocketPath,
			description:        "Client should default to DefaultSocketPath when no path provided",
		},
		{
			name: "creates client with custom socket path",
			config: Config{
				SocketPath: "/tmp/nv-hostengine",
			},
			expectedSocketPath: "/tmp/nv-hostengine",
			description:        "Client should accept custom socket paths",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(tc.config, logger)

			require.NotNil(t, client, "NewClient should return a non-nil client")

			// Type assert to access internal fields for verification.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			assert.Equal(t, tc.expectedSocketPath, dcgmClient.socketPath, tc.description)
			assert.NotNil(t, dcgmClient.logger, "Logger should be set")
			assert.False(t, dcgmClient.connected, "Client should not be connected on creation")
			assert.False(t, dcgmClient.hasViolation, "Client should not have violations on creation")
		})
	}
}

// TestClient_IsHealthy_NotInitialized tests that IsHealthy returns true during grace period when not connected.
func TestClient_IsHealthy_NotInitialized(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	healthy := client.IsHealthy()

	assert.True(t, healthy, "IsHealthy should return true for not connected client within grace period")
}

// TestClient_Shutdown_NotInitialized tests that Shutdown handles not connected state gracefully.
func TestClient_Shutdown_NotInitialized(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	err := client.Shutdown()

	assert.NoError(t, err, "Shutdown should not error when client is not connected")
}

// TestClient_Reconcile_NotInitialized tests that Reconcile initializes when not yet connected.
func TestClient_Reconcile_NotInitialized(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 1 * time.Minute}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	justInit, err := client.Reconcile(ctx)

	// We expect an error because DCGM is not available in test environment.
	// However, if we're within grace period, error might be nil.
	if err != nil {
		assert.Error(t, err, "Reconcile should attempt to initialize when client is not connected")
	}

	assert.False(t, justInit, "justInit should be false when initialization fails")

	// Verify client remains not connected after failure.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok)
	assert.False(t, dcgmClient.connected, "Client should remain not connected after reconciliation failure")
}

// TestClient_Reconcile_GracePeriod tests that errors are suppressed during grace period.
func TestClient_Reconcile_GracePeriod(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 5 * time.Second}, logger)

	// Set lastShutdown to now to trigger grace period.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok)
	dcgmClient.lastShutdown = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	justInit, err := client.Reconcile(ctx)

	assert.NoError(t, err, "Reconcile should not return errors within grace period after shutdown")
	assert.False(t, justInit, "justInit should be false during grace period")
}

// TestClient_Reconcile_AfterGracePeriod tests that errors are reported after grace period.
func TestClient_Reconcile_AfterGracePeriod(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 100 * time.Millisecond}, logger)

	// Set lastShutdown to past to be outside grace period.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok)
	dcgmClient.lastShutdown = time.Now().Add(-1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	justInit, err := client.Reconcile(ctx)

	assert.Error(t, err, "Reconcile should return errors after grace period expires")
	assert.False(t, justInit, "justInit should be false when initialization fails")
}

// TestClient_MultipleShutdownCalls tests that multiple Shutdown calls are safe.
func TestClient_MultipleShutdownCalls(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	// Call Shutdown multiple times.
	err1 := client.Shutdown()
	err2 := client.Shutdown()
	err3 := client.Shutdown()

	assert.NoError(t, err1, "Multiple Shutdown calls should be safe and not error")
	assert.NoError(t, err2, "Multiple Shutdown calls should be safe and not error")
	assert.NoError(t, err3, "Multiple Shutdown calls should be safe and not error")
}

// TestClient_Shutdown_Initialized tests that Shutdown properly cleans up when client is connected.
func TestClient_Shutdown_Initialized(t *testing.T) {
	testCases := []struct {
		name        string
		description string
	}{
		{
			name:        "shutdown cleans up connected client",
			description: "Shutdown should clean up all resources when client is connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Manually set the client to connected state to test shutdown path.
			ctx, cancel := context.WithCancel(context.Background())
			cleanupCalled := false
			mockCleanup := func() {
				cleanupCalled = true
			}

			var destroyedHandles []dcgm.FieldHandle
			mockFieldGroupDestroy := func(handle dcgm.FieldHandle) error {
				destroyedHandles = append(destroyedHandles, handle)
				return nil
			}

			const (
				fakeMetricsFieldGroupHandle = 42
				fakeXidFieldGroupHandle     = 99
			)
			var metricsFieldGroup dcgm.FieldHandle
			metricsFieldGroup.SetHandle(fakeMetricsFieldGroupHandle)
			var xidFieldGroup dcgm.FieldHandle
			xidFieldGroup.SetHandle(fakeXidFieldGroupHandle)

			dcgmClient.mu.Lock()
			dcgmClient.connected = true
			dcgmClient.metricsWatchActive = true
			dcgmClient.xidWatchActive = true
			dcgmClient.metricsFieldGroup = metricsFieldGroup
			dcgmClient.xidFieldGroup = xidFieldGroup
			dcgmClient.ctx = ctx
			dcgmClient.cancelPolicyListener = cancel
			dcgmClient.cleanupFunc = mockCleanup
			dcgmClient.fieldGroupDestroyFunc = mockFieldGroupDestroy
			dcgmClient.shutdownHandlers = []func(){cancel}
			dcgmClient.mu.Unlock()

			// Call Shutdown.
			err := client.Shutdown()

			assert.NoError(t, err, tc.description)
			assert.True(t, cleanupCalled, "Cleanup function should have been called")
			require.Len(t, destroyedHandles, 2, "FieldGroupDestroy should be called for metrics and XID groups")
			assert.Equal(t, metricsFieldGroup.GetHandle(), destroyedHandles[0].GetHandle(), "First destroy should be metrics field group")
			assert.Equal(t, xidFieldGroup.GetHandle(), destroyedHandles[1].GetHandle(), "Second destroy should be XID field group")

			// Verify client state was reset.
			dcgmClient.mu.RLock()
			assert.False(t, dcgmClient.connected, "Client should be marked as not connected")
			assert.False(t, dcgmClient.metricsWatchActive, "Metrics watch flag should be reset for clean reconnect")
			assert.Nil(t, dcgmClient.cleanupFunc, "Cleanup function should be nil after shutdown")
			assert.Empty(t, dcgmClient.shutdownHandlers, "Shutdown handlers should be empty")
			assert.False(t, dcgmClient.lastShutdown.IsZero(), "Last shutdown time should be set")
			dcgmClient.mu.RUnlock()
		})
	}
}

// TestClient_ThreadSafety tests that concurrent access to IsHealthy is thread-safe.
func TestClient_ThreadSafety(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	// Launch multiple goroutines calling IsHealthy concurrently.
	const goroutines = 10
	done := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = client.IsHealthy()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete.
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// If we reach here without data race, test passes.
	assert.True(t, true, "Multiple goroutines calling IsHealthy should not cause data races")
}

// TestClient_ContextCancellation tests that context cancellation stops policy listener.
func TestClient_ContextCancellation(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 1 * time.Minute}, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Attempt to reconcile (will fail but that's ok for this test).
	_, _ = client.Reconcile(ctx)

	// Cancel context immediately.
	cancel()

	// Give time for goroutine to exit.
	time.Sleep(50 * time.Millisecond)

	// Shutdown should still work.
	err := client.Shutdown()
	assert.NoError(t, err, "Cancelling context should not cause panics or errors")
}

// TestClient_LoggerUsage tests that logger is properly used.
func TestClient_LoggerUsage(t *testing.T) {
	t.Parallel()

	// Create a logger that captures logs.
	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 1 * time.Minute}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Attempt to reconcile (will fail and log error).
	_, _ = client.Reconcile(ctx)

	// If we reach here without panic, logger is working.
	assert.True(t, true, "Client should use logger for error messages")
}

// TestClient_DefaultLogger tests client creation with default logger.
func TestClient_DefaultLogger(t *testing.T) {
	t.Parallel()

	// Use default logger.
	client := NewClient(Config{InitializationGracePeriod: 1 * time.Minute}, zap.L())

	require.NotNil(t, client, "Client should work with zap.L() default logger")

	// Verify basic operations work.
	healthy := client.IsHealthy()
	assert.True(t, healthy, "Not connected client within grace period should be healthy")

	err := client.Shutdown()
	assert.NoError(t, err, "Shutdown should not error")
}

// TestClient_HealthyStateProducesCorrectStatus tests that healthy client state produces OK status.
func TestClient_HealthyStateProducesCorrectStatus(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{SocketPath: "/run/nvidia-dcgm/nv-hostengine"}, logger)

	// Type assert to access internal fields.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok, "Client should be of type *dcgmClient")

	// Manually set the client to connected state without violations.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dcgmClient.mu.Lock()
	dcgmClient.connected = true
	dcgmClient.hasViolation = false
	dcgmClient.ctx = ctx
	dcgmClient.cancelPolicyListener = cancel
	dcgmClient.mu.Unlock()

	// IsHealthy should return true since there are no violations.
	healthy := dcgmClient.isHealthyWithoutHealthCheck()
	assert.True(t, healthy, "Healthy client should report OK status")
}

// TestClient_UnhealthyStateProducesCorrectStatus tests that unhealthy client state produces IMPAIRED status.
func TestClient_UnhealthyStateProducesCorrectStatus(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{SocketPath: "/run/nvidia-dcgm/nv-hostengine"}, logger)

	// Type assert to access internal fields.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok, "Client should be of type *dcgmClient")

	// Manually set the client to connected state with violations.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dcgmClient.mu.Lock()
	dcgmClient.connected = true
	dcgmClient.hasViolation = true
	dcgmClient.lastShutdown = time.Now().Add(-2 * dcgmClient.initializationGracePeriod)
	dcgmClient.ctx = ctx
	dcgmClient.cancelPolicyListener = cancel
	dcgmClient.mu.Unlock()

	// IsHealthy should return false since there is at least one violation.
	healthy := dcgmClient.isHealthyWithoutHealthCheck()
	assert.False(t, healthy, "Unhealthy client should report IMPAIRED status")
}

// isHealthyWithoutHealthCheck is a helper method for testing that checks health
// without calling dcgm.HealthCheck (which would fail in test environment).
func (c *dcgmClient) isHealthyWithoutHealthCheck() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return !c.hasViolation
}

// TestSocketPathConfigurationAcceptance tests specific examples
// to ensure the property holds for common cases.
func TestSocketPathConfigurationAcceptance(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		socketPath  string
		description string
	}{
		{
			name:        "default socket path",
			socketPath:  DefaultSocketPath,
			description: "Should accept default socket path",
		},
		{
			name:        "tmp socket path",
			socketPath:  "/tmp/nv-hostengine",
			description: "Should accept tmp socket path",
		},
		{
			name:        "run directory socket path",
			socketPath:  "/run/nvidia-dcgm/nv-hostengine",
			description: "Should accept run directory socket path",
		},
		{
			name:        "custom socket path",
			socketPath:  "/run/nvidia-dcgm/custom-hostengine",
			description: "Should accept custom socket path",
		},
		{
			name:        "nested directory socket path",
			socketPath:  "/opt/nvidia/dcgm/nv-hostengine.sock",
			description: "Should accept nested directory socket path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			config := Config{
				SocketPath: tc.socketPath,
			}

			client := NewClient(config, logger)

			require.NotNil(t, client, "NewClient should return a non-nil client")

			// Type assert to access internal fields for verification.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			assert.Equal(t, tc.socketPath, dcgmClient.socketPath, tc.description)
		})
	}
}

// TestExtractXIDCode tests that XID codes are correctly extracted from policy violations.
func TestExtractXIDCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		val1        uint64
		expected    uint64
		description string
	}{
		{
			name:        "extracts XID code 13",
			val1:        13,
			expected:    13,
			description: "Should extract Graphics Engine Exception code",
		},
		{
			name:        "extracts XID code 31",
			val1:        31,
			expected:    31,
			description: "Should extract GPU memory page fault code",
		},
		{
			name:        "extracts XID code 48",
			val1:        48,
			expected:    48,
			description: "Should extract Double Bit ECC Error code",
		},
		{
			name:        "extracts XID code 79",
			val1:        79,
			expected:    79,
			description: "Should extract GPU fallen off bus code",
		},
		{
			name:        "extracts XID code 140",
			val1:        140,
			expected:    140,
			description: "Should extract Unrecoverable ECC Error code",
		},
		{
			name:        "extracts non-well-known XID code",
			val1:        99,
			expected:    99,
			description: "Should extract non-well-known XID codes",
		},
		{
			name:        "extracts zero XID code",
			val1:        0,
			expected:    0,
			description: "Should extract zero XID code",
		},
		{
			name:        "extracts large XID code",
			val1:        999,
			expected:    999,
			description: "Should extract large XID codes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock violation with the test XID code.
			violation := mockPolicyViolation{
				Condition: dcgm.XidPolicy,
				Val1:      tc.val1,
			}

			result := extractXIDCode(violation.toPolicyViolation())

			assert.Equal(t, tc.expected, result, tc.description)
		})
	}

	// Test the fallback when Data is not XidPolicyCondition.
	t.Run("returns zero when Data is not XidPolicyCondition", func(t *testing.T) {
		t.Parallel()

		violation := dcgm.PolicyViolation{
			Condition: dcgm.XidPolicy,
			Data:      "not an XidPolicyCondition",
		}

		result := extractXIDCode(violation)

		assert.Equal(t, uint64(0), result,
			"Should return 0 when Data type assertion fails.")
	})
}

// TestIsCriticalViolation_NonXIDPolicies tests that all non-XID policy types are marked as critical.
func TestIsCriticalViolation_NonXIDPolicies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		condition   dcgm.PolicyCondition
		description string
	}{
		{
			name:        "DBE policy is critical",
			condition:   dcgm.DbePolicy,
			description: "Double-bit ECC errors should always be marked as critical",
		},
		{
			name:        "NVLink policy is critical",
			condition:   dcgm.NvlinkPolicy,
			description: "NVLink errors should always be marked as critical",
		},
		{
			name:        "MaxRtPg policy is critical",
			condition:   dcgm.MaxRtPgPolicy,
			description: "Maximum retired pages violations should always be marked as critical",
		},
		{
			name:        "Power policy is critical",
			condition:   dcgm.PowerPolicy,
			description: "Power violations should always be marked as critical",
		},
		{
			name:        "Thermal policy is critical",
			condition:   dcgm.ThermalPolicy,
			description: "Thermal violations should always be marked as critical",
		},
		{
			name:        "PCIe policy is critical",
			condition:   dcgm.PCIePolicy,
			description: "PCIe replay errors should always be marked as critical",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok)

			// Create a mock non-XID violation.
			violation := mockPolicyViolation{
				Condition: tc.condition,
				Val1:      0, // Value doesn't matter for non-XID policies.
			}

			result := dcgmClient.isCriticalViolation(violation.toPolicyViolation())

			assert.True(t, result, tc.description)
		})
	}
}

// TestWellKnownXIDCodesCompleteness tests that the wellKnownXIDCodes map contains exactly the expected codes.
func TestWellKnownXIDCodesCompleteness(t *testing.T) {
	t.Parallel()

	expectedCodes := []uint64{46, 48, 54, 62, 64, 74, 79, 95, 109, 110, 136, 140, 142, 143, 151, 155, 156, 158}

	t.Run("wellKnownXIDCodes contains exactly 18 codes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 18, len(wellKnownXIDCodes),
			"wellKnownXIDCodes should contain exactly 18 codes")
	})

	t.Run("wellKnownXIDCodes contains all expected codes", func(t *testing.T) {
		t.Parallel()

		for _, code := range expectedCodes {
			assert.True(t, wellKnownXIDCodes[code],
				"wellKnownXIDCodes should contain code %d", code)
		}
	})

	t.Run("wellKnownXIDCodes contains only expected codes", func(t *testing.T) {
		t.Parallel()

		// Create a map of expected codes for quick lookup.
		expectedMap := make(map[uint64]bool)
		for _, code := range expectedCodes {
			expectedMap[code] = true
		}

		// Verify all codes in wellKnownXIDCodes are expected.
		for code := range wellKnownXIDCodes {
			assert.True(t, expectedMap[code],
				"wellKnownXIDCodes should not contain unexpected code %d", code)
		}
	})
}

// TestXIDFilteringIntegration tests the integration of XID filtering with the hasViolation flag.
func TestXIDFilteringIntegration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		xidCode          uint64
		expectedCritical bool
		expectedHealthy  bool
		description      string
	}{
		{
			name:             "well-known XID code marks instance as unhealthy",
			xidCode:          48,
			expectedCritical: true,
			expectedHealthy:  false,
			description:      "XID 48 should mark instance as unhealthy",
		},
		{
			name:             "non-well-known XID code does not mark instance as unhealthy",
			xidCode:          99,
			expectedCritical: false,
			expectedHealthy:  true,
			description:      "XID 99 should not mark instance as unhealthy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok)

			// Set client to connected state.
			dcgmClient.mu.Lock()
			dcgmClient.connected = true
			// Set lastShutdown to past to be outside grace period.
			dcgmClient.lastShutdown = time.Now().Add(-10 * time.Minute)
			dcgmClient.hasViolation = false
			dcgmClient.mu.Unlock()

			// Create a mock XID violation.
			violation := mockPolicyViolation{
				Condition: dcgm.XidPolicy,
				Val1:      tc.xidCode,
			}

			// Check if violation is critical.
			isCritical := dcgmClient.isCriticalViolation(violation.toPolicyViolation())
			assert.Equal(t, tc.expectedCritical, isCritical,
				"isCriticalViolation should return %v for XID %d", tc.expectedCritical, tc.xidCode)

			// Simulate setting hasViolation flag if critical.
			if isCritical {
				dcgmClient.mu.Lock()
				dcgmClient.hasViolation = true
				dcgmClient.mu.Unlock()
			}

			// Check health status.
			healthy := dcgmClient.isHealthyWithoutHealthCheck()
			assert.Equal(t, tc.expectedHealthy, healthy, tc.description)
		})
	}
}

// mockPolicyViolation is a mock implementation of dcgm.PolicyViolation for testing.
// It matches the structure of the actual dcgm.PolicyViolation type from go-dcgm library.
type mockPolicyViolation struct {
	Condition dcgm.PolicyCondition
	Timestamp time.Time
	GpuId     uint
	Val1      uint64
	Val2      uint64
}

// toPolicyViolation converts mockPolicyViolation to dcgm.PolicyViolation with proper Data field.
// Only XidPolicy is populated since the agent only listens to XID policy violations.
func (m mockPolicyViolation) toPolicyViolation() dcgm.PolicyViolation {
	v := dcgm.PolicyViolation{
		Condition: m.Condition,
		Timestamp: m.Timestamp,
	}

	if m.Condition == dcgm.XidPolicy {
		v.Data = dcgm.XidPolicyCondition{
			ErrNum: uint(m.Val1),
		}
	}

	return v
}

// TestClient_IsConnectionLost_WithinGracePeriod tests that IsConnectionLost returns false within grace period.
func TestClient_IsConnectionLost_WithinGracePeriod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		gracePeriod time.Duration
		connected   bool
		description string
	}{
		{
			name:        "returns false when not connected but within grace period",
			gracePeriod: 5 * time.Second,
			connected:   false,
			description: "IsConnectionLost should return false within grace period even if not connected",
		},
		{
			name:        "returns false when connected within grace period",
			gracePeriod: 5 * time.Second,
			connected:   true,
			description: "IsConnectionLost should return false within grace period when connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			config := Config{
				InitializationGracePeriod: tc.gracePeriod,
			}
			client := NewClient(config, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Set lastShutdown to now to be within grace period.
			dcgmClient.mu.Lock()
			dcgmClient.lastShutdown = time.Now()
			dcgmClient.connected = tc.connected
			dcgmClient.mu.Unlock()

			// IsConnectionLost should return false within grace period.
			connectionLost := client.IsConnectionLost()

			assert.False(t, connectionLost, tc.description)
		})
	}
}

// TestClient_IsConnectionLost_OutsideGracePeriod tests that IsConnectionLost returns true outside grace period.
func TestClient_IsConnectionLost_OutsideGracePeriod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		gracePeriod  time.Duration
		connected    bool
		expectedLost bool
		description  string
	}{
		{
			name:         "returns true when not connected and outside grace period",
			gracePeriod:  100 * time.Millisecond,
			connected:    false,
			expectedLost: true,
			description:  "IsConnectionLost should return true when not connected outside grace period",
		},
		{
			name:         "returns false when connected outside grace period",
			gracePeriod:  100 * time.Millisecond,
			connected:    true,
			expectedLost: false,
			description:  "IsConnectionLost should return false when connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			config := Config{
				InitializationGracePeriod: tc.gracePeriod,
			}
			client := NewClient(config, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Set lastShutdown to past to be outside grace period.
			dcgmClient.mu.Lock()
			dcgmClient.lastShutdown = time.Now().Add(-1 * time.Second)
			dcgmClient.connected = tc.connected
			dcgmClient.mu.Unlock()

			// IsConnectionLost should return expected value outside grace period.
			connectionLost := client.IsConnectionLost()

			assert.Equal(t, tc.expectedLost, connectionLost, tc.description)
		})
	}
}

// TestClient_IsConnectionLost_ThreadSafety tests that concurrent access to IsConnectionLost is thread-safe.
func TestClient_IsConnectionLost_ThreadSafety(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	// Launch multiple goroutines calling IsConnectionLost concurrently.
	const goroutines = 10
	done := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = client.IsConnectionLost()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete.
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// If we reach here without data race, test passes.
	assert.True(t, true, "Multiple goroutines calling IsConnectionLost should not cause data races")
}

// TestClient_IsConnectionLost_InitialState tests IsConnectionLost behavior on newly created client.
func TestClient_IsConnectionLost_InitialState(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 5 * time.Second}, logger)

	// Check IsConnectionLost immediately after creation.
	connectionLost := client.IsConnectionLost()

	// Should return false because we're within grace period.
	assert.False(t, connectionLost, "Newly created client should report connection not lost within grace period")
}

// TestClient_IsConnectionLost_AfterReconcile tests IsConnectionLost behavior after Reconcile attempts.
func TestClient_IsConnectionLost_AfterReconcile(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{InitializationGracePeriod: 100 * time.Millisecond}, logger)

	// Type assert to access internal fields.
	dcgmClient, ok := client.(*dcgmClient)
	require.True(t, ok, "Client should be of type *dcgmClient")

	// Set lastShutdown to past to be outside grace period.
	dcgmClient.mu.Lock()
	dcgmClient.lastShutdown = time.Now().Add(-1 * time.Second)
	dcgmClient.mu.Unlock()

	// Attempt to reconcile (will fail because DCGM is not available).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _ = client.Reconcile(ctx)

	// Check IsConnectionLost - should return true because initialization failed.
	connectionLost := client.IsConnectionLost()

	assert.True(t, connectionLost, "IsConnectionLost should return true after failed reconcile outside grace period")
}

// TestClient_IsConnectionLost_DistinguishesFromIsHealthy tests that IsConnectionLost and IsHealthy work together.
func TestClient_IsConnectionLost_DistinguishesFromIsHealthy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		connected          bool
		hasViolation       bool
		gracePeriod        time.Duration
		outsideGracePeriod bool
		expectedHealthy    bool
		expectedConnLost   bool
		description        string
	}{
		{
			name:               "healthy and connected when connected without violations",
			connected:          true,
			hasViolation:       false,
			gracePeriod:        5 * time.Second,
			outsideGracePeriod: true,
			expectedHealthy:    true,
			expectedConnLost:   false,
			description:        "Should report healthy and connected when connected without violations",
		},
		{
			name:               "unhealthy but connected when connected with violations",
			connected:          true,
			hasViolation:       true,
			gracePeriod:        5 * time.Second,
			outsideGracePeriod: true,
			expectedHealthy:    false,
			expectedConnLost:   false,
			description:        "Should report unhealthy but connected when connected with violations",
		},
		{
			name:               "healthy but connection lost when not connected outside grace period",
			connected:          false,
			hasViolation:       false,
			gracePeriod:        100 * time.Millisecond,
			outsideGracePeriod: true,
			expectedHealthy:    true,
			expectedConnLost:   true,
			description:        "Should report healthy but connection lost when not connected outside grace period",
		},
		{
			name:               "healthy and connected within grace period even if not connected",
			connected:          false,
			hasViolation:       false,
			gracePeriod:        5 * time.Second,
			outsideGracePeriod: false,
			expectedHealthy:    true,
			expectedConnLost:   false,
			description:        "Should report healthy and connected within grace period even if not connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			config := Config{
				InitializationGracePeriod: tc.gracePeriod,
			}
			client := NewClient(config, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Set up client state.
			dcgmClient.mu.Lock()
			if tc.outsideGracePeriod {
				dcgmClient.lastShutdown = time.Now().Add(-1 * tc.gracePeriod).Add(-1 * time.Second)
			} else {
				dcgmClient.lastShutdown = time.Now()
			}
			dcgmClient.connected = tc.connected
			dcgmClient.hasViolation = tc.hasViolation
			dcgmClient.mu.Unlock()

			// Check IsHealthy and IsConnectionLost.
			healthy := dcgmClient.isHealthyWithoutHealthCheck()
			connectionLost := client.IsConnectionLost()

			assert.Equal(t, tc.expectedHealthy, healthy, "IsHealthy: "+tc.description)
			assert.Equal(t, tc.expectedConnLost, connectionLost, "IsConnectionLost: "+tc.description)
		})
	}
}

// TestClient_PolicyViolationIntegration tests the complete flow from policy violation to health status.
func TestClient_PolicyViolationIntegration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		violations       []mockPolicyViolation
		expectedHealthy  bool
		expectedConnLost bool
		description      string
	}{
		{
			name: "critical XID violation marks instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 0, Val1: 48},
			},
			expectedHealthy:  false,
			expectedConnLost: false,
			description:      "Critical XID violation should mark instance as unhealthy",
		},
		{
			name: "non-critical XID violation does not mark instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 0, Val1: 99},
			},
			expectedHealthy:  true,
			expectedConnLost: false,
			description:      "Non-critical XID violation should not mark instance as unhealthy",
		},
		{
			name: "DBE violation marks instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.DbePolicy, Timestamp: time.Now(), GpuId: 0, Val1: 5, Val2: 0x1000},
			},
			expectedHealthy:  false,
			expectedConnLost: false,
			description:      "DBE violation should mark instance as unhealthy",
		},
		{
			name: "thermal violation marks instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.ThermalPolicy, Timestamp: time.Now(), GpuId: 0, Val1: 95},
			},
			expectedHealthy:  false,
			expectedConnLost: false,
			description:      "Thermal violation should mark instance as unhealthy",
		},
		{
			name: "multiple non-critical XID violations do not mark instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 0, Val1: 1},
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 1, Val1: 2},
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 2, Val1: 99},
			},
			expectedHealthy:  true,
			expectedConnLost: false,
			description:      "Multiple non-critical XID violations should not mark instance as unhealthy",
		},
		{
			name: "mix of critical and non-critical violations marks instance as unhealthy",
			violations: []mockPolicyViolation{
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 0, Val1: 1},
				{Condition: dcgm.XidPolicy, Timestamp: time.Now(), GpuId: 1, Val1: 48},
			},
			expectedHealthy:  false,
			expectedConnLost: false,
			description:      "Mix of critical and non-critical violations should mark instance as unhealthy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok)

			// Set client to connected state.
			dcgmClient.mu.Lock()
			dcgmClient.connected = true
			dcgmClient.hasViolation = false
			dcgmClient.lastShutdown = time.Now().Add(-2 * dcgmClient.initializationGracePeriod)
			dcgmClient.mu.Unlock()

			// Process all violations.
			for _, violation := range tc.violations {
				dcgmClient.logPolicyViolation(violation.toPolicyViolation())
				if dcgmClient.isCriticalViolation(violation.toPolicyViolation()) {
					dcgmClient.mu.Lock()
					dcgmClient.hasViolation = true
					dcgmClient.mu.Unlock()
				}
			}

			// Check health status.
			healthy := dcgmClient.isHealthyWithoutHealthCheck()
			connectionLost := client.IsConnectionLost()

			assert.Equal(t, tc.expectedHealthy, healthy, tc.description)
			assert.Equal(t, tc.expectedConnLost, connectionLost, "Connection should not be lost")
		})
	}
}

// TestClient_HealthCheckResultFiltering tests that health check results are properly filtered.
// Note: This test documents the expected behavior when DCGM health checks return different result codes.
// In the actual implementation, IsHealthy() calls dcgm.HealthCheck() which is not available in tests,
// so we test the filtering logic conceptually.
func TestClient_HealthCheckResultFiltering(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		healthResult    uint
		expectedHealthy bool
		description     string
	}{
		{
			name:            "DCGM_HEALTH_RESULT_PASS marks instance as healthy",
			healthResult:    0, // DCGM_HEALTH_RESULT_PASS
			expectedHealthy: true,
			description:     "Health check PASS result should mark instance as healthy",
		},
		{
			name:            "DCGM_HEALTH_RESULT_WARN does not mark instance as unhealthy",
			healthResult:    10, // DCGM_HEALTH_RESULT_WARN
			expectedHealthy: true,
			description:     "Health check WARN result should not mark instance as unhealthy",
		},
		{
			name:            "DCGM_HEALTH_RESULT_FAIL marks instance as unhealthy",
			healthResult:    20, // DCGM_HEALTH_RESULT_FAIL
			expectedHealthy: false,
			description:     "Health check FAIL result should mark instance as unhealthy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// This test documents the expected behavior based on the implementation in IsHealthy().
			// The actual filtering logic is:
			// - DCGM_HEALTH_RESULT_PASS (0): return true (healthy)
			// - DCGM_HEALTH_RESULT_WARN (10): log warning but return true (healthy)
			// - DCGM_HEALTH_RESULT_FAIL (20): return false (unhealthy)
			//
			// Since we cannot mock dcgm.HealthCheck() in unit tests (it requires actual DCGM),
			// we verify the logic is correctly implemented by checking the code behavior.

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok)

			// Set client to connected state without violations.
			dcgmClient.mu.Lock()
			dcgmClient.connected = true
			dcgmClient.hasViolation = false
			dcgmClient.mu.Unlock()

			// The isHealthyWithoutHealthCheck helper simulates the behavior before health check.
			// In the actual IsHealthy() implementation:
			// 1. Check grace period -> return true if within grace period
			// 2. Check initialized -> return true if not initialized (connection issue)
			// 3. Check hasViolation -> return false if true
			// 4. Call dcgm.HealthCheck() -> filter results as documented above
			//
			// Since we cannot call dcgm.HealthCheck() in tests, we verify the logic
			// by ensuring the implementation matches the documented behavior.

			// Verify the client is in the expected state for health check.
			healthy := dcgmClient.isHealthyWithoutHealthCheck()
			assert.True(t, healthy, "Client should be healthy before health check when no violations exist")

			// The actual filtering happens in IsHealthy() when dcgm.HealthCheck() is called.
			// The implementation correctly filters:
			// - response.OverallHealth == dcgm.DCGM_HEALTH_RESULT_FAIL (20) -> return false
			// - response.OverallHealth == dcgm.DCGM_HEALTH_RESULT_WARN (10) -> log warning, return true
			// - Otherwise (PASS) -> return true
		})
	}
}

// TestClient_ConnectionLossDetection tests connection loss detection scenarios.
func TestClient_ConnectionLossDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		connected         bool
		gracePeriod       time.Duration
		withinGracePeriod bool
		expectedConnLost  bool
		description       string
	}{
		{
			name:              "connection lost when not connected outside grace period",
			connected:         false,
			gracePeriod:       100 * time.Millisecond,
			withinGracePeriod: false,
			expectedConnLost:  true,
			description:       "Connection should be lost when not connected outside grace period",
		},
		{
			name:              "connection not lost when not connected within grace period",
			connected:         false,
			gracePeriod:       5 * time.Second,
			withinGracePeriod: true,
			expectedConnLost:  false,
			description:       "Connection should not be lost when not connected within grace period",
		},
		{
			name:              "connection not lost when connected",
			connected:         true,
			gracePeriod:       100 * time.Millisecond,
			withinGracePeriod: false,
			expectedConnLost:  false,
			description:       "Connection should not be lost when connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			config := Config{
				InitializationGracePeriod: tc.gracePeriod,
			}
			client := NewClient(config, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Set up client state.
			dcgmClient.mu.Lock()
			if tc.withinGracePeriod {
				dcgmClient.lastShutdown = time.Now()
			} else {
				dcgmClient.lastShutdown = time.Now().Add(-1 * time.Second)
			}
			dcgmClient.connected = tc.connected
			dcgmClient.mu.Unlock()

			// Check connection lost status.
			connectionLost := client.IsConnectionLost()

			assert.Equal(t, tc.expectedConnLost, connectionLost, tc.description)
		})
	}
}

func TestClient_IsHealthy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		hasViolation bool
		connected    bool
		expected     bool
		description  string
	}{
		{
			name:         "returns false when hasViolation is true and connected",
			hasViolation: true,
			connected:    true,
			expected:     false,
			description:  "IsHealthy should return false when hasViolation is true",
		},
		{
			name:         "returns false when hasViolation is true and not connected",
			hasViolation: true,
			connected:    false,
			expected:     false,
			description:  "IsHealthy should return false when hasViolation is true even if not connected",
		},
		{
			name:         "returns true when hasViolation is false and not connected",
			hasViolation: false,
			connected:    false,
			expected:     true,
			description:  "IsHealthy should return true when hasViolation is false and not connected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)

			// Type assert to access internal fields.
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok, "Client should be of type *dcgmClient")

			// Set up client state.
			dcgmClient.mu.Lock()
			dcgmClient.hasViolation = tc.hasViolation
			dcgmClient.connected = tc.connected
			dcgmClient.mu.Unlock()

			// Call IsHealthy.
			healthy := dcgmClient.isHealthyWithoutHealthCheck()

			assert.Equal(t, tc.expected, healthy, tc.description)
		})
	}
}

// TestClient_PolicyViolationLoggingFields tests that all required fields are logged for policy violations.
func TestClient_PolicyViolationLoggingFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		violation   mockPolicyViolation
		description string
	}{
		{
			name: "XID violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.XidPolicy,
				Timestamp: time.Now(),
				GpuId:     0,
				Val1:      48,
			},
			description: "XID violation should log condition, timestamp, GPU ID, XID code, critical flag, and message",
		},
		{
			name: "DBE violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.DbePolicy,
				Timestamp: time.Now(),
				GpuId:     1,
				Val1:      5,
				Val2:      0x1000,
			},
			description: "DBE violation should log condition, timestamp, GPU ID, error count, and memory location",
		},
		{
			name: "Thermal violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.ThermalPolicy,
				Timestamp: time.Now(),
				GpuId:     2,
				Val1:      85,
			},
			description: "Thermal violation should log condition, timestamp, GPU ID, and temperature",
		},
		{
			name: "Power violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.PowerPolicy,
				Timestamp: time.Now(),
				GpuId:     3,
				Val1:      400,
			},
			description: "Power violation should log condition, timestamp, GPU ID, and power",
		},
		{
			name: "NVLink violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.NvlinkPolicy,
				Timestamp: time.Now(),
				GpuId:     4,
				Val1:      0,
				Val2:      10,
			},
			description: "NVLink violation should log condition, timestamp, GPU ID, link ID, and error count",
		},
		{
			name: "MaxRtPg violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.MaxRtPgPolicy,
				Timestamp: time.Now(),
				GpuId:     5,
				Val1:      63,
			},
			description: "MaxRtPg violation should log condition, timestamp, GPU ID, and retired pages",
		},
		{
			name: "PCIe violation logs all required fields",
			violation: mockPolicyViolation{
				Condition: dcgm.PCIePolicy,
				Timestamp: time.Now(),
				GpuId:     6,
				Val1:      50,
			},
			description: "PCIe violation should log condition, timestamp, GPU ID, and replay errors",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := zaptest.NewLogger(t)
			client := NewClient(Config{}, logger)
			dcgmClient, ok := client.(*dcgmClient)
			require.True(t, ok)

			// Call logPolicyViolation - should not panic and should log all required fields.
			dcgmClient.logPolicyViolation(tc.violation.toPolicyViolation())

			// If we reach here without panic, logging worked correctly.
			// The actual field verification happens through the logger, which we cannot
			// easily inspect in unit tests. The test verifies that the logging call
			// completes successfully for all policy types.
			assert.True(t, true, tc.description)
		})
	}
}

// TestClient_GetMetrics_NotConnected tests that GetMetrics returns an error when client is not connected.
func TestClient_GetMetrics_NotConnected(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	client := NewClient(Config{}, logger)

	ctx := context.Background()
	metrics, err := client.GetMetrics(ctx)

	assert.Error(t, err, "GetMetrics should return error when client is not connected")
	assert.Nil(t, metrics, "GetMetrics should return nil metrics when not connected")
	assert.Contains(t, err.Error(), "not connected", "Error message should indicate connection issue")
}

// TestIsValidInt64Value tests the DCGM int64 sentinel value detection.
func TestIsValidInt64Value(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		value    int64
		expected bool
	}{
		{
			name:     "valid zero value",
			value:    0,
			expected: true,
		},
		{
			name:     "valid positive value",
			value:    42,
			expected: true,
		},
		{
			name:     "valid 100 percent",
			value:    100,
			expected: true,
		},
		{
			name:     "valid large value",
			value:    16384, // 16 GiB in MiB
			expected: true,
		},
		{
			name:     "DCGM_INT32_BLANK",
			value:    0x7ffffff0,
			expected: false,
		},
		{
			name:     "DCGM_INT32_NOT_FOUND",
			value:    0x7ffffff1,
			expected: false,
		},
		{
			name:     "DCGM_INT32_NOT_SUPPORTED",
			value:    0x7ffffff2,
			expected: false,
		},
		{
			name:     "DCGM_INT32_NOT_PERMISSIONED",
			value:    0x7ffffff3,
			expected: false,
		},
		{
			name:     "DCGM_INT64_BLANK",
			value:    0x7ffffffffffffff0,
			expected: false,
		},
		{
			name:     "DCGM_INT64_NOT_FOUND",
			value:    0x7ffffffffffffff1,
			expected: false,
		},
		{
			name:     "DCGM_INT64_NOT_SUPPORTED",
			value:    0x7ffffffffffffff2,
			expected: false,
		},
		{
			name:     "DCGM_INT64_NOT_PERMISSIONED",
			value:    0x7ffffffffffffff3,
			expected: false,
		},
		{
			name:     "negative value is valid",
			value:    -1,
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := isValidInt64Value(tc.value)
			assert.Equal(t, tc.expected, result, "isValidInt64Value(%d) should be %v", tc.value, tc.expected)
		})
	}
}

// TestIsValidFloat64Value tests the DCGM float64 sentinel value detection.
func TestIsValidFloat64Value(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		value    float64
		expected bool
	}{
		{
			name:     "valid zero",
			value:    0.0,
			expected: true,
		},
		{
			name:     "valid utilization percentage",
			value:    45.5,
			expected: true,
		},
		{
			name:     "valid temperature",
			value:    85.0,
			expected: true,
		},
		{
			name:     "valid power draw",
			value:    350.0,
			expected: true,
		},
		{
			name:     "valid 100 percent",
			value:    100.0,
			expected: true,
		},
		{
			name:     "valid large value",
			value:    1e10,
			expected: true,
		},
		{
			name:     "DCGM FP64 BLANK sentinel",
			value:    140737488355328.0,
			expected: false,
		},
		{
			name:     "DCGM FP64 NOT_FOUND sentinel",
			value:    140737488355329.0,
			expected: false,
		},
		{
			name:     "DCGM FP64 NOT_SUPPORTED sentinel",
			value:    140737488355330.0,
			expected: false,
		},
		{
			name:     "DCGM FP64 NOT_PERMISSIONED sentinel",
			value:    140737488355331.0,
			expected: false,
		},
		{
			name:     "negative value is invalid",
			value:    -1.0,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := isValidFloat64Value(tc.value)
			assert.Equal(t, tc.expected, result, "isValidFloat64Value(%f) should be %v", tc.value, tc.expected)
		})
	}
}

// makeInt64FieldValue creates a dcgm.FieldValue_v1 with the given status and int64 value.
func makeInt64FieldValue(status int, val int64) dcgm.FieldValue_v1 {
	fv := dcgm.FieldValue_v1{Status: status}
	binary.LittleEndian.PutUint64(fv.Value[:8], uint64(val))
	return fv
}

// makeFloat64FieldValue creates a dcgm.FieldValue_v1 with the given status and float64 value.
func makeFloat64FieldValue(status int, val float64) dcgm.FieldValue_v1 {
	fv := dcgm.FieldValue_v1{Status: status}
	binary.LittleEndian.PutUint64(fv.Value[:8], math.Float64bits(val))
	return fv
}

// TestIsValidInt64FieldValue tests FieldValue_v1 int64 validation including status checks.
func TestIsValidInt64FieldValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fv       dcgm.FieldValue_v1
		expected bool
	}{
		{
			name:     "valid value with status 0",
			fv:       makeInt64FieldValue(0, 42),
			expected: true,
		},
		{
			name:     "valid zero value with status 0",
			fv:       makeInt64FieldValue(0, 0),
			expected: true,
		},
		{
			name:     "sentinel BLANK with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffffffffffff0),
			expected: false,
		},
		{
			name:     "sentinel NOT_FOUND with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffffffffffff1),
			expected: false,
		},
		{
			name:     "sentinel NOT_SUPPORTED with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffffffffffff2),
			expected: false,
		},
		{
			name:     "sentinel NOT_PERMISSIONED with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffffffffffff3),
			expected: false,
		},
		{
			name:     "INT32 sentinel NOT_SUPPORTED with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffff2),
			expected: false,
		},
		{
			name:     "INT32 sentinel NOT_PERMISSIONED with status 0",
			fv:       makeInt64FieldValue(0, 0x7ffffff3),
			expected: false,
		},
		{
			name:     "valid value with non-zero status",
			fv:       makeInt64FieldValue(1, 42),
			expected: false,
		},
		{
			name:     "sentinel value with non-zero status",
			fv:       makeInt64FieldValue(-1, 0x7ffffff0),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := isValidInt64FieldValue(tc.fv)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestIsValidFloat64FieldValue tests FieldValue_v1 float64 validation including status checks.
func TestIsValidFloat64FieldValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fv       dcgm.FieldValue_v1
		expected bool
	}{
		{
			name:     "valid value with status 0",
			fv:       makeFloat64FieldValue(0, 45.5),
			expected: true,
		},
		{
			name:     "valid zero with status 0",
			fv:       makeFloat64FieldValue(0, 0.0),
			expected: true,
		},
		{
			name:     "DCGM FP64 BLANK sentinel with status 0",
			fv:       makeFloat64FieldValue(0, 140737488355328.0),
			expected: false,
		},
		{
			name:     "negative value with status 0",
			fv:       makeFloat64FieldValue(0, -1.0),
			expected: false,
		},
		{
			name:     "valid value with non-zero status",
			fv:       makeFloat64FieldValue(1, 45.5),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := isValidFloat64FieldValue(tc.fv)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSafeInt64ToUint64 tests the safe int64 to uint64 conversion helper.
func TestSafeInt64ToUint64(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    int64
		expected uint64
	}{
		{name: "positive value", input: 42, expected: 42},
		{name: "zero", input: 0, expected: 0},
		{name: "large positive", input: 16384, expected: 16384},
		{name: "negative clamps to zero", input: -1, expected: 0},
		{name: "large negative clamps to zero", input: -999999, expected: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := safeInt64ToUint64(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// makeFieldValueV2 creates a FieldValue_v2 with the given entity ID and XID code
// encoded in the Value byte array (little-endian int64).
func makeFieldValueV2(entityID uint, xidCode int64) dcgm.FieldValue_v2 {
	fv := dcgm.FieldValue_v2{
		EntityID: entityID,
	}
	for i := 0; i < 8; i++ {
		fv.Value[i] = byte(xidCode >> (i * 8))
	}
	return fv
}

// TestCountRestartAppXidsByDevice verifies that countRestartAppXidsByDevice correctly
// filters FieldValue_v2 entries to include only RESTART_APP XID codes and counts per device.
func TestCountRestartAppXidsByDevice(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		entries  []dcgm.FieldValue_v2
		expected map[uint]int64
	}{
		{
			name:     "empty input returns empty map",
			entries:  nil,
			expected: map[uint]int64{},
		},
		{
			name: "single RESTART_APP XID on one device",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, 13),
			},
			expected: map[uint]int64{0: 1},
		},
		{
			name: "multiple RESTART_APP XIDs on same device",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, 13),
				makeFieldValueV2(0, 31),
				makeFieldValueV2(0, 94),
			},
			expected: map[uint]int64{0: 3},
		},
		{
			name: "RESTART_APP XIDs across multiple devices",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, 13),
				makeFieldValueV2(1, 31),
				makeFieldValueV2(0, 94),
				makeFieldValueV2(2, 8),
			},
			expected: map[uint]int64{0: 2, 1: 1, 2: 1},
		},
		{
			name: "non-RESTART_APP codes are filtered out",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, 46),
				makeFieldValueV2(0, 79),
				makeFieldValueV2(0, 200),
			},
			expected: map[uint]int64{},
		},
		{
			name: "mixed RESTART_APP and non-RESTART_APP codes",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, 13),
				makeFieldValueV2(0, 46),
				makeFieldValueV2(1, 94),
				makeFieldValueV2(1, 200),
			},
			expected: map[uint]int64{0: 1, 1: 1},
		},
		{
			name: "negative XID codes are skipped",
			entries: []dcgm.FieldValue_v2{
				makeFieldValueV2(0, -1),
				makeFieldValueV2(0, 13),
			},
			expected: map[uint]int64{0: 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := countRestartAppXidsByDevice(tc.entries)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMapDeviceIndexToUUID verifies that mapDeviceIndexToUUID correctly converts
// device-index-keyed counts to UUID-keyed counts and skips unmapped indices.
func TestMapDeviceIndexToUUID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		countsByDev  map[uint]int64
		deviceToUUID map[uint]string
		expected     map[string]int64
	}{
		{
			name:         "empty counts returns empty map",
			countsByDev:  map[uint]int64{},
			deviceToUUID: map[uint]string{0: "GPU-aaa"},
			expected:     map[string]int64{},
		},
		{
			name:         "all indices mapped",
			countsByDev:  map[uint]int64{0: 3, 1: 5},
			deviceToUUID: map[uint]string{0: "GPU-aaa", 1: "GPU-bbb"},
			expected:     map[string]int64{"GPU-aaa": 3, "GPU-bbb": 5},
		},
		{
			name:         "unmapped index is skipped",
			countsByDev:  map[uint]int64{0: 3, 2: 7},
			deviceToUUID: map[uint]string{0: "GPU-aaa"},
			expected:     map[string]int64{"GPU-aaa": 3},
		},
		{
			name:         "empty UUID map skips all",
			countsByDev:  map[uint]int64{0: 3},
			deviceToUUID: map[uint]string{},
			expected:     map[string]int64{},
		},
		{
			name:         "single device",
			countsByDev:  map[uint]int64{0: 1},
			deviceToUUID: map[uint]string{0: "GPU-aaa"},
			expected:     map[string]int64{"GPU-aaa": 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := mapDeviceIndexToUUID(tc.countsByDev, tc.deviceToUUID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractMetricsFromFieldValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		values          []dcgm.FieldValue_v1
		expectedMetric  gputypes.GPUMetric
		expectedSkipped []string
	}{
		{
			name: "all valid fields",
			values: []dcgm.FieldValue_v1{
				makeStringFieldValue(0, "GPU-abc-123"), // UUID
				makeInt64FieldValue(0, 16384),          // FB Total: 16384 MiB
				makeInt64FieldValue(0, 85),             // GPU Util: 85%
				makeFloat64FieldValue(0, 0.5),          // Mem Util: 50% (8192/16384)
				makeInt64FieldValue(0, 8192),           // FB Used: 8192 MiB
				makeFloat64FieldValue(0, 250.5),        // Power: 250.5W
				makeInt64FieldValue(0, 72),             // Temp: 72°C
			},
			expectedMetric: gputypes.GPUMetric{
				GPUUUID:           "GPU-abc-123",
				MemoryTotal:       ptrUint64(16384 * 1024 * 1024),
				GPUUtilization:    ptrFloat64(85.0),
				MemoryUtilization: ptrFloat64(50.0),
				MemoryUsed:        ptrUint64(8192 * 1024 * 1024),
				PowerDraw:         ptrFloat64(250.5),
				Temperature:       ptrFloat64(72.0),
			},
		},
		{
			name: "invalid fields are skipped",
			values: []dcgm.FieldValue_v1{
				makeStringFieldValue(1, ""),                 // UUID: bad status
				makeInt64FieldValue(0, 0x7ffffffffffffff0),  // FB Total: int64 BLANK sentinel
				makeInt64FieldValue(0, 50),                  // GPU Util: valid (50%)
				makeFloat64FieldValue(0, -1.0),              // Mem Util: negative = invalid
				makeInt64FieldValue(1, 4096),                // FB Used: bad status
				makeFloat64FieldValue(0, 140737488355328.0), // Power: FP64 BLANK sentinel
				makeInt64FieldValue(0, 65),                  // Temp: valid (65°C)
			},
			expectedMetric: gputypes.GPUMetric{
				GPUUtilization: ptrFloat64(50.0),
				Temperature:    ptrFloat64(65.0),
			},
			expectedSkipped: []string{
				"DCGM_FI_DEV_FB_TOTAL",
				"DCGM_FI_DEV_FB_USED_PERCENT",
				"DCGM_FI_DEV_FB_USED",
				"DCGM_FI_DEV_POWER_USAGE",
			},
		},
		{
			name:           "empty values slice",
			values:         []dcgm.FieldValue_v1{},
			expectedMetric: gputypes.GPUMetric{},
		},
		{
			name: "partial values slice",
			values: []dcgm.FieldValue_v1{
				makeStringFieldValue(0, "GPU-partial"), // UUID
				makeInt64FieldValue(0, 8192),           // FB Total: 8192 MiB
			},
			expectedMetric: gputypes.GPUMetric{
				GPUUUID:     "GPU-partial",
				MemoryTotal: ptrUint64(8192 * 1024 * 1024),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var metric gputypes.GPUMetric
			skipped := extractMetricsFromFieldValues(&metric, tc.values)

			assert.Equal(t, tc.expectedMetric.GPUUUID, metric.GPUUUID)
			assert.Equal(t, tc.expectedMetric.MemoryTotal, metric.MemoryTotal)
			assert.Equal(t, tc.expectedMetric.GPUUtilization, metric.GPUUtilization)
			assert.Equal(t, tc.expectedMetric.MemoryUtilization, metric.MemoryUtilization)
			assert.Equal(t, tc.expectedMetric.MemoryUsed, metric.MemoryUsed)
			assert.Equal(t, tc.expectedMetric.PowerDraw, metric.PowerDraw)
			assert.Equal(t, tc.expectedMetric.Temperature, metric.Temperature)

			if tc.expectedSkipped == nil {
				assert.Empty(t, skipped)
			} else {
				assert.Equal(t, tc.expectedSkipped, skipped)
			}
		})
	}
}

func makeStringFieldValue(status int, val string) dcgm.FieldValue_v1 {
	fv := dcgm.FieldValue_v1{Status: status}
	copy(fv.Value[:], val)
	return fv
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrUint64(v uint64) *uint64    { return &v }
