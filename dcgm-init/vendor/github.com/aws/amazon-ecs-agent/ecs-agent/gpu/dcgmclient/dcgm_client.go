//go:build linux

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

package gpu

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"go.uber.org/zap"
)

const (
	// DefaultSocketPath is the default Unix domain socket path for the DCGM nv-hostengine.
	DefaultSocketPath = "/run/nvidia-dcgm/nv-hostengine"

	// DefaultInitializationGracePeriod is the default duration after shutdown during which
	// initialization failures are not reported as errors.
	DefaultInitializationGracePeriod = 3 * time.Minute

	// mibToBytes converts MiB to bytes. DCGM reports framebuffer memory in MiB.
	mibToBytes = 1024 * 1024
)

// metricsFieldDef pairs a DCGM field ID with its human-readable name.
// This is the single source of truth for the basic GPU metrics we watch.
type metricsFieldDef struct {
	id   dcgm.Short
	name string
}

// metricsFields defines the DCGM fields for GPU metrics collection.
// These fields are watched once during initialization and queried on each collection tick
// via GetLatestValuesForFields(), avoiding the expensive per-call create/watch/destroy
// cycle that GetDeviceStatus() performs internally.
//
// IMPORTANT: The position of each entry must match its corresponding fieldIdx* constant.
// Reordering entries without updating the iota block will cause extractMetricsFromFieldValues
// to read the wrong field at each index.
var metricsFields = []metricsFieldDef{
	{dcgm.DCGM_FI_DEV_UUID, "DCGM_FI_DEV_UUID"},                       // fieldIdxUUID
	{dcgm.DCGM_FI_DEV_FB_TOTAL, "DCGM_FI_DEV_FB_TOTAL"},               // fieldIdxFBTotal
	{dcgm.DCGM_FI_DEV_GPU_UTIL, "DCGM_FI_DEV_GPU_UTIL"},               // fieldIdxGPUUtil
	{dcgm.DCGM_FI_DEV_FB_USED_PERCENT, "DCGM_FI_DEV_FB_USED_PERCENT"}, // fieldIdxMemUtil
	{dcgm.DCGM_FI_DEV_FB_USED, "DCGM_FI_DEV_FB_USED"},                 // fieldIdxFBUsed
	{dcgm.DCGM_FI_DEV_POWER_USAGE, "DCGM_FI_DEV_POWER_USAGE"},         // fieldIdxPower
	{dcgm.DCGM_FI_DEV_GPU_TEMP, "DCGM_FI_DEV_GPU_TEMP"},               // fieldIdxTemp
}

// metricsFieldIDs returns the DCGM field IDs derived from metricsFields.
func metricsFieldIDs() []dcgm.Short {
	ids := make([]dcgm.Short, len(metricsFields))
	for i, f := range metricsFields {
		ids[i] = f.id
	}
	return ids
}

// Field index constants for metricsFields. Must match the order above.
const (
	fieldIdxUUID = iota
	fieldIdxFBTotal
	fieldIdxGPUUtil
	fieldIdxMemUtil
	fieldIdxFBUsed
	fieldIdxPower
	fieldIdxTemp
)

// wellKnownXIDCodes contains XID codes that indicate serious hardware failures
// requiring instance drain. These codes are aligned with EKSNodeMonitoringAgent.
//
// XID codes represent GPU hardware exceptions reported by the NVIDIA driver.
// The well-known codes include:
//   - 46: GPU stopped processing (display-related).
//   - 48: Double Bit ECC Error (uncorrectable memory error, requires GPU reset).
//   - 54: Auxiliary power connector not connected.
//   - 62: Internal micro-controller halt (requires GPU reset).
//   - 64: GPU memory remapping failure (ECC error handling failed).
//   - 74: NVLink Error (GPU-to-GPU or GPU-to-NVSwitch connection failure).
//   - 79: GPU has fallen off the bus (PCIe link failure).
//   - 95: Uncontained memory error (error affects multiple applications).
//   - 109: Context switch timeout (GPU context switch took too long).
//   - 110: GPU disappeared from the bus (PCIe link failure).
//   - 136: GPU memory page retirement limit exceeded.
//   - 140: Unrecoverable ECC Error (uncorrectable memory error).
//   - 142: GPU memory page retired due to uncorrectable error.
//   - 143: GPU memory page retired due to correctable error threshold.
//   - 151: GPU to CPU interconnect error.
//   - 155: GPU NVLink flit CRC error.
//   - 156: GPU NVLink lane error.
//   - 158: GPU InfoROM corrupted.
//
// Reference: https://docs.nvidia.com/deploy/xid-errors/index.html
var wellKnownXIDCodes = map[uint64]bool{
	46:  true,
	48:  true,
	54:  true,
	62:  true,
	64:  true,
	74:  true,
	79:  true,
	95:  true,
	109: true,
	110: true,
	136: true,
	140: true,
	142: true,
	143: true,
	151: true,
	155: true,
	156: true,
	158: true,
}

// restartAppXIDCodes contains XID codes in the RESTART_APP resolution bucket.
// When these occur, NVIDIA recommends restarting the application.
// These are customer-actionable errors reported as GPURestartAppXidCount.
// Reference: https://docs.nvidia.com/deploy/xid-errors/index.html
var restartAppXIDCodes = map[uint64]bool{
	8: true, 11: true, 13: true, 25: true, 31: true, 32: true,
	39: true, 40: true, 41: true, 60: true, 68: true, 69: true,
	70: true, 71: true, 72: true, 75: true, 76: true, 77: true,
	80: true, 82: true, 83: true, 84: true, 85: true, 86: true,
	88: true, 89: true, 94: true, 96: true, 97: true, 98: true,
	99: true, 100: true, 101: true, 102: true, 103: true, 104: true,
	105: true, 126: true, 127: true, 128: true, 129: true, 130: true,
	131: true, 132: true, 133: true, 134: true, 135: true, 139: true,
}

// Client provides an interface for monitoring Nvidia GPU health through the
// Nvidia vended Data Center GPU Monitor (DCGM).
type Client interface {
	// Reconcile performs health check on existing connection and reconnects if needed.
	// Returns true if DCGM was reinitialized, false if connection was already healthy.
	// Returns error if reconnection fails outside the grace period.
	Reconcile(ctx context.Context) (bool, error)

	// IsHealthy returns true if all GPUs are healthy, false otherwise.
	// Checks for policy violations and health check errors.
	IsHealthy() bool

	// IsConnectionLost returns true if the DCGM connection is lost and outside grace period.
	// This is used to distinguish between connection issues (UNKNOWN status)
	// and actual hardware failures (IMPAIRED status).
	//
	// Connection is considered lost when:
	// - Not initialized AND outside grace period, OR
	// - Introspection fails during Reconcile (detected by connectionLost flag)
	IsConnectionLost() bool

	// GetMetrics collects GPU telemetry metrics from all supported devices.
	// Returns a slice of GPUMetric, one per GPU device, including per-GPU RESTART_APP XID counts.
	// Individual metric fields may be nil if the metric is unavailable for a device.
	// Returns an error if the client is not connected or DCGM communication fails.
	GetMetrics(ctx context.Context) ([]gputypes.GPUMetric, error)

	// Shutdown cleans up DCGM resources and closes connections.
	Shutdown() error

	// UnhealthyReason returns the reason the instance was marked unhealthy.
	// Returns an empty string if no critical violation has occurred.
	UnhealthyReason() string
}

// Config holds configuration for the DCGM client.
type Config struct {
	// SocketPath is the Unix domain socket path for the nv-hostengine (e.g., "/run/nvidia-dcgm/nv-hostengine").
	// If empty, defaults to DefaultSocketPath.
	SocketPath string

	// InitializationGracePeriod is the duration after shutdown during which
	// initialization failures are not reported as errors. This prevents noisy
	// errors during expected disconnects or DCGM restarts.
	// If zero, defaults to DefaultInitializationGracePeriod.
	InitializationGracePeriod time.Duration
}

// dcgmClient implements the Client interface.
type dcgmClient struct {
	// Unix domain socket path for nv-hostengine (e.g., "/run/nvidia-dcgm/nv-hostengine").
	socketPath string

	// Grace period after shutdown before reporting initialization errors.
	initializationGracePeriod time.Duration

	// Timestamp of last shutdown for grace period calculation.
	lastShutdown time.Time

	// Context for lifecycle management.
	ctx context.Context

	// Cancel function for policy violation listener.
	cancelPolicyListener context.CancelFunc

	// Channel for receiving policy violations.
	policyViolationChan <-chan dcgm.PolicyViolation

	// Cleanup function returned by dcgm.Init.
	cleanupFunc func()

	// Shutdown handlers to call during cleanup (e.g., cancel functions).
	shutdownHandlers []func()

	// Flag indicating if DCGM connection is active.
	connected bool

	// Flag indicating if any policy violation has occurred.
	// Only violations (filtered XID codes, non-XID policies) set this flag.
	hasViolation bool

	// Reason the instance was marked unhealthy (e.g., "XID_48").
	// Empty string means no critical XID violation has occurred.
	unhealthyReason string

	// metricsFieldGroup is a DCGM field group created once during initialization and
	// reused across collection ticks. A "field watch" tells nv-hostengine to continuously
	// sample a set of DCGM fields (e.g. GPU utilization, temperature) at a configured
	// frequency. Once watched, the latest values can be read cheaply via
	// GetLatestValuesForFields() without any group creation or teardown per call.
	metricsFieldGroup dcgm.FieldHandle

	// metricsWatchActive indicates whether the metrics field watch is active.
	// Set to true after successful WatchFieldsWithGroupEx() during initialization.
	metricsWatchActive bool

	// xidFieldGroup is the DCGM field group for DCGM_FI_DEV_XID_ERRORS.
	xidFieldGroup dcgm.FieldHandle

	// xidWatchActive indicates whether the XID field watch is active.
	xidWatchActive bool

	// lastXidQueryTime is the cursor for GetValuesSince.
	// Initialized to current time during setup; updated to nextSinceTimestamp after each query.
	lastXidQueryTime time.Time

	// deviceIndexToUUID maps GPU device index to UUID. Populated during GetMetrics.
	deviceIndexToUUID map[uint]string

	// fieldGroupDestroyFunc is the function used to destroy DCGM field groups.
	// Defaults to dcgm.FieldGroupDestroy; overridable in tests.
	fieldGroupDestroyFunc func(dcgm.FieldHandle) error

	// Mutex for thread-safe access to state.
	mu sync.RWMutex

	// Number of pending dcgm.Init attempts to prevent goroutine leaks.
	pendingInitAttempts atomic.Int32

	// Logger for debugging and error reporting.
	logger *zap.Logger
}

// NewClient creates a new DCGM client with the given configuration.
func NewClient(config Config, logger *zap.Logger) Client {
	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	gracePeriod := config.InitializationGracePeriod
	if gracePeriod == 0 {
		gracePeriod = DefaultInitializationGracePeriod
	}

	return &dcgmClient{
		socketPath:                socketPath,
		initializationGracePeriod: gracePeriod,
		lastShutdown:              time.Now(),
		fieldGroupDestroyFunc:     dcgm.FieldGroupDestroy,
		logger:                    logger,
	}
}

// Reconcile performs health check and reconnects if needed.
// Returns (justInitialized, error) where:
//   - justInitialized is true if DCGM was reinitialized during this call, false if connection was already healthy.
//   - error is non-nil if reconnection fails outside the grace period, nil otherwise.
func (c *dcgmClient) Reconcile(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("reconciling DCGM connection",
		zap.Bool("connected", c.connected),
		zap.Bool("hasViolation", c.hasViolation))

	// If connected, perform health check via introspection.
	if c.connected {
		_, introspectErr := dcgm.Introspect()
		if introspectErr == nil {
			c.logger.Info("DCGM connection healthy, no reconnection needed")
			return false, nil
		}
		c.logger.Warn("DCGM health check failed, will attempt reconnection", zap.Error(introspectErr))
		c.connected = false // Mark connection as lost.
	} else {
		c.logger.Info("DCGM not connected, will attempt to connect")
	}

	// Shutdown existing connection before reinitializing.
	if err := c.shutdownLocked(); err != nil {
		return false, fmt.Errorf("failed to shutdown DCGM: %w", err)
	}

	// Attempt to connect to DCGM.
	if err := c.initializeLocked(ctx); err != nil {
		// Check if we're within grace period after shutdown.
		timeSinceShutdown := time.Since(c.lastShutdown)
		if time.Now().Before(c.lastShutdown.Add(c.initializationGracePeriod)) {
			c.logger.Info("initialization failed within grace period, suppressing error",
				zap.Duration("gracePeriod", c.initializationGracePeriod),
				zap.Duration("timeSinceShutdown", timeSinceShutdown),
				zap.Error(err))
			return false, nil
		}
		c.logger.Error("initialization failed outside grace period",
			zap.Duration("timeSinceShutdown", timeSinceShutdown),
			zap.Error(err))
		return false, fmt.Errorf("failed to initialize DCGM: %w", err)
	}

	c.logger.Info("DCGM reinitialized successfully")
	return true, nil
}

// initializeLocked connects to DCGM and sets up monitoring.
// Caller must hold c.mu lock.
func (c *dcgmClient) initializeLocked(ctx context.Context) error {
	c.logger.Info("initializing DCGM client", zap.String("socketPath", c.socketPath))

	// Check if there's already a pending initialization attempt to prevent unbounded go routine creation.
	// We use CompareAndSwap to atomically check and increment, ensuring only one goroutine proceeds.
	if !c.pendingInitAttempts.CompareAndSwap(0, 1) {
		c.logger.Warn("initialization already in progress, skipping new attempt",
			zap.Int32("pendingAttempts", c.pendingInitAttempts.Load()))
		return fmt.Errorf("DCGM initialization already in progress")
	}

	// Initialize DCGM in standalone mode to connect to nv-hostengine.
	// Use a timeout to prevent blocking indefinitely when host engine is unavailable.
	c.logger.Debug("connecting to host engine", zap.String("socketPath", c.socketPath))

	// dcgm.Init() is a blocking call that connects to nv-hostengine via Unix domain socket.
	// The underlying NVIDIA DCGM C library does not support context cancellation or
	// configurable timeouts. When nv-hostengine is unavailable, the connection
	// attempt can block for extended periods.
	//
	// Our mitigation strategy is:
	//   1. Run dcgm.Init() in a goroutine to avoid blocking the caller.
	//   2. Use a 10-second timeout to fail fast when nv-hostengine is unavailable.
	//   3. Track pending attempts with pendingInitAttempts atomic counter.
	//   4. Limit to 1 concurrent attempt to prevent unbounded goroutine creation.
	//
	// This bounds the resource creation to a single goroutine that will eventually
	// terminate. It's a local endpoint so it's very unlikely to take more than
	// 10 seconds to succeed.
	type initResult struct {
		cleanup func()
		err     error
	}
	resultChan := make(chan initResult, 1)

	go func() {
		// Decrement counter when goroutine completes (success or failure).
		defer c.pendingInitAttempts.Add(-1)

		// The second parameter is the socket path and the third parameter "1" indicates
		// Unix domain socket connection mode rather than TCP/IP. This connects to
		// nv-hostengine via the specified Unix domain socket.
		cleanup, err := dcgm.Init(dcgm.Standalone, c.socketPath, "1")
		resultChan <- initResult{cleanup: cleanup, err: err}
	}()

	// Wait for initialization with timeout.
	const initTimeout = 10 * time.Second
	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, initTimeout)
	defer cancelTimeout()

	var result initResult
	select {
	case result = <-resultChan:
		// Initialization completed.
		if result.err != nil {
			c.logger.Error("failed to connect to host engine",
				zap.String("socketPath", c.socketPath),
				zap.Error(result.err))
			return result.err
		}
	case <-timeoutCtx.Done():
		c.logger.Error("timeout connecting to host engine",
			zap.String("socketPath", c.socketPath),
			zap.Duration("timeout", initTimeout))
		return fmt.Errorf("timeout connecting to nv-hostengine after %v", initTimeout)
	}

	c.cleanupFunc = result.cleanup
	c.logger.Info("successfully connected to host engine")

	// Create context for policy violation listener.
	policyCtx, cancelPolicy := context.WithCancel(ctx)
	c.ctx = policyCtx
	c.cancelPolicyListener = cancelPolicy
	c.shutdownHandlers = append(c.shutdownHandlers, cancelPolicy)

	// Register policy violation listeners for all required policies.
	// These policies monitor critical GPU health indicators that signal hardware degradation or failure.
	c.logger.Info("registering policy violation listeners")
	policyChan, err := dcgm.ListenForPolicyViolations(policyCtx,
		// XidPolicy: GPU hardware exceptions (XID errors).
		// Example: XID 48 indicates a double-bit ECC error, XID 79 means
		// the GPU fell off the PCIe bus (often due to power issues or bad connection).
		dcgm.XidPolicy,
	)
	if err != nil {
		c.logger.Error("failed to register policy listeners", zap.Error(err))
		c.shutdownHandlers = nil
		cancelPolicy()
		if c.cleanupFunc != nil {
			c.cleanupFunc()
		}
		return err
	}
	c.logger.Info("successfully registered policy violation listeners")

	c.policyViolationChan = policyChan

	// Enable all DCGM health check systems. We consider the instance to be unhealthy even if a GPU
	// that is not in use is impaired. This prevents a situation where a task is launched and given
	// an impaired GPU.
	c.logger.Info("enabling health check systems for all GPUs")
	if err := dcgm.HealthSet(dcgm.GroupAllGPUs(), dcgm.DCGM_HEALTH_WATCH_ALL); err != nil {
		c.logger.Error("failed to enable health check systems", zap.Error(err))
		c.shutdownHandlers = nil
		cancelPolicy()
		if c.cleanupFunc != nil {
			c.cleanupFunc()
		}
		return err
	}
	c.logger.Info("successfully enabled health check systems")

	// Start goroutine to listen for policy violations.
	go c.listenForPolicyViolations()

	// Mark as connected. Note: We do NOT reset hasViolation here because
	// policy violations should persist across DCGM reconnections. Once a GPU
	// reports a violation (thermal, power, XID error, etc.), it indicates
	// hardware degradation that doesn't go away just because DCGM restarted.
	// The violation flag should only be cleared when explicitly reset or when
	// the client is first created.
	c.connected = true

	// Set up persistent metrics field watch for basic GPU metrics.
	c.setupMetricsWatches()

	// Set up persistent XID error field watch for per-GPU XID counting.
	c.setupXidWatch()

	c.logger.Info("DCGM client initialized successfully",
		zap.String("socketPath", c.socketPath),
		zap.Bool("metricsWatchActive", c.metricsWatchActive),
		zap.Bool("xidWatchActive", c.xidWatchActive))

	return nil
}

// listenForPolicyViolations monitors the policy violation channel and sets hasViolation flag.
func (c *dcgmClient) listenForPolicyViolations() {
	c.logger.Info("policy violation listener started")
	for {
		select {
		case <-c.ctx.Done():
			// Context cancelled, exit goroutine.
			c.logger.Info("policy violation listener stopped")
			return
		case violation, ok := <-c.policyViolationChan:
			if !ok {
				// Channel closed, exit goroutine.
				c.logger.Info("policy violation channel closed")
				return
			}

			// Log detailed violation information.
			c.logPolicyViolation(violation)

			// Filter violations to determine if they are critical.
			// Only critical violations (well-known XID codes or non-XID policies) mark the instance as unhealthy.
			if c.isCriticalViolation(violation) {
				c.mu.Lock()
				c.hasViolation = true
				if violation.Condition == dcgm.XidPolicy && c.unhealthyReason == "" {
					c.unhealthyReason = fmt.Sprintf("XID_%d", extractXIDCode(violation))
				}
				c.mu.Unlock()
			}
		}
	}
}

// IsHealthy returns true if GPUs are healthy.
// The caller should check IsConnectionLost() to determine if UNKNOWN status should be reported.
func (c *dcgmClient) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.logger.Debug("checking GPU health status",
		zap.Bool("connected", c.connected),
		zap.Bool("hasViolation", c.hasViolation))

	// Return false if any policy violation has occurred.
	if c.hasViolation {
		c.logger.Warn("policy violation detected, reporting unhealthy")
		return false
	}

	// We don't have a connection to DCGM so we don't want to run the health checks again.
	// Return true because we don't know of any violations.
	if !c.connected {
		return true
	}

	// Perform health check and filter results.
	// DCGM health check returns a response with an overall result. The potential values are:
	//   - DCGM_HEALTH_RESULT_PASS
	//   - DCGM_HEALTH_RESULT_WARN
	//   - DCGM_HEALTH_RESULT_FAIL
	//
	// We only mark the instance as unhealthy for FAIL results.
	// WARN results indicate non-critical issues that don't require instance draining.
	response, err := dcgm.HealthCheck(dcgm.GroupAllGPUs())
	if err != nil {
		c.logger.Error("health check failed", zap.Error(err))
		// Health check failure is a connection/communication issue, not hardware failure.
		// The caller should check IsConnectionLost() to handle this properly.
		return true
	}

	if response.OverallHealth == dcgm.DCGM_HEALTH_RESULT_FAIL {
		c.logger.Warn("health check returned FAIL status",
			zap.Uint("overallHealth", uint(response.OverallHealth)))
		return false
	}

	if response.OverallHealth == dcgm.DCGM_HEALTH_RESULT_WARN {
		c.logger.Warn("health check returned WARN status (non-critical)",
			zap.Uint("overallHealth", uint(response.OverallHealth)))
	}

	return true
}

// IsConnectionLost returns true if the DCGM connection is lost and outside grace period.
// This is used to distinguish between connection issues (UNKNOWN status)
// and actual hardware failures (IMPAIRED status).
//
// Connection is considered lost when not connected AND outside grace period.
//
// Within the grace period this method returns false to avoid
// reporting UNKNOWN status during expected DCGM service restarts.
func (c *dcgmClient) IsConnectionLost() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Within grace period, don't report connection as lost.
	if time.Now().Before(c.lastShutdown.Add(c.initializationGracePeriod)) {
		return false
	}

	// Outside grace period: connection is lost if not connected.
	return !c.connected
}

// UnhealthyReason returns the reason the instance was marked unhealthy.
// Returns an empty string if no critical XID violation has occurred.
func (c *dcgmClient) UnhealthyReason() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.unhealthyReason
}

// collectXidCounts queries DCGM for XID events since the given timestamp, filters for
// RESTART_APP codes, counts per device, maps to UUID, and updates the cursor.
// Returns an empty map if the query fails.
func (c *dcgmClient) collectXidCounts(sinceTime time.Time, deviceToUUID map[uint]string) map[string]int64 {
	entries, nextSinceTimestamp, err := dcgm.GetValuesSince(dcgm.GroupAllGPUs(), c.xidFieldGroup, sinceTime)
	if err != nil {
		c.logger.Error("failed to get XID values since last query, returning empty counts", zap.Error(err))
		return make(map[string]int64)
	}

	countsByDevice := countRestartAppXidsByDevice(entries)
	countsByUUID := mapDeviceIndexToUUID(countsByDevice, deviceToUUID)

	c.mu.Lock()
	c.lastXidQueryTime = nextSinceTimestamp
	c.mu.Unlock()

	return countsByUUID
}

// Shutdown cleans up DCGM resources.
func (c *dcgmClient) Shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.shutdownLocked()
}

// shutdownLocked cleans up DCGM resources.
// Caller must hold c.mu lock.
func (c *dcgmClient) shutdownLocked() error {
	// If not connected, nothing to clean up.
	if !c.connected {
		c.logger.Debug("DCGM client not connected, nothing to shutdown")
		return nil
	}

	c.logger.Info("shutting down DCGM client")

	// Call all shutdown handlers (e.g., cancel functions).
	for _, handler := range c.shutdownHandlers {
		handler()
	}
	c.shutdownHandlers = nil

	// Destroy field groups before disconnecting to free names on nv-hostengine.
	if c.metricsWatchActive {
		if err := c.fieldGroupDestroyFunc(c.metricsFieldGroup); err != nil {
			c.logger.Debug("failed to destroy metrics field group", zap.Error(err))
		}
	}
	if c.xidWatchActive {
		if err := c.fieldGroupDestroyFunc(c.xidFieldGroup); err != nil {
			c.logger.Debug("failed to destroy XID field group", zap.Error(err))
		}
	}

	// Call cleanup function to disconnect from host engine.
	if c.cleanupFunc != nil {
		c.cleanupFunc()
		c.cleanupFunc = nil
		c.logger.Debug("DCGM cleanup function called successfully")
	}

	// Reset state.
	c.connected = false
	c.metricsWatchActive = false
	c.xidWatchActive = false
	c.lastXidQueryTime = time.Time{}
	c.deviceIndexToUUID = nil
	c.lastShutdown = time.Now()
	c.logger.Info("DCGM client shutdown successfully")

	return nil
}

// GetMetrics collects GPU telemetry metrics from all supported devices.
// Returns a slice of GPUMetric, one per GPU. Individual metric fields may be nil
// if the persistent field watch is not active or a field value is unavailable.
func (c *dcgmClient) GetMetrics(ctx context.Context) ([]gputypes.GPUMetric, error) {
	c.mu.RLock()
	connected := c.connected
	metricsWatchActive := c.metricsWatchActive
	xidWatchActive := c.xidWatchActive
	lastXidQueryTime := c.lastXidQueryTime
	c.mu.RUnlock()

	if !connected {
		return nil, fmt.Errorf("DCGM client is not connected")
	}

	// Get list of supported GPU devices.
	gpus, err := dcgm.GetSupportedDevices()
	if err != nil {
		c.logger.Error("failed to get supported devices", zap.Error(err))
		return nil, fmt.Errorf("failed to get supported devices: %w", err)
	}

	c.logger.Debug("collecting metrics for GPU devices", zap.Int("deviceCount", len(gpus)))

	metrics := make([]gputypes.GPUMetric, len(gpus))
	for i, gpu := range gpus {
		// Query all metrics (including UUID and total memory) via the persistent field watch.
		if metricsWatchActive {
			values, err := dcgm.GetLatestValuesForFields(gpu, metricsFieldIDs())
			if err != nil {
				c.logger.Warn("failed to get latest field values, skipping device",
					zap.Uint("deviceIndex", gpu),
					zap.Error(err))
				continue
			}

			if skipped := extractMetricsFromFieldValues(&metrics[i], values); len(skipped) > 0 {
				c.logger.Debug("sentinel values detected for metric fields",
					zap.Uint("deviceIndex", gpu),
					zap.String("gpuUUID", metrics[i].GPUUUID),
					zap.Strings("skippedFields", skipped))
			}
		}
	}

	c.logger.Debug("collected GPU metrics",
		zap.Int("deviceCount", len(metrics)),
		zap.Bool("metricsWatchActive", metricsWatchActive))

	// Build device index to UUID mapping and collect per-GPU XID counts.
	deviceToUUID := make(map[uint]string, len(gpus))
	for i, gpu := range gpus {
		if metrics[i].GPUUUID != "" {
			deviceToUUID[gpu] = metrics[i].GPUUUID
		}
	}

	// Collect per-GPU RESTART_APP XID counts via GetValuesSince.
	if xidWatchActive {
		xidCounts := c.collectXidCounts(lastXidQueryTime, deviceToUUID)
		for i := range metrics {
			if count, ok := xidCounts[metrics[i].GPUUUID]; ok {
				metrics[i].RestartAppXidCount = count
			}
		}
	}

	c.mu.Lock()
	c.deviceIndexToUUID = deviceToUUID
	c.mu.Unlock()

	return metrics, nil
}

// extractMetricsFromFieldValues populates a GPUMetric from raw DCGM field values.
// The values slice must correspond to metricsFields in the same order.
// Returns the names of fields that were skipped due to sentinel or invalid values.
func extractMetricsFromFieldValues(metric *gputypes.GPUMetric, values []dcgm.FieldValue_v1) []string {
	var skipped []string

	if fieldIdxUUID < len(values) && values[fieldIdxUUID].Status == 0 {
		metric.GPUUUID = values[fieldIdxUUID].String()
	}
	if fieldIdxFBTotal < len(values) {
		if isValidInt64FieldValue(values[fieldIdxFBTotal]) {
			v := safeInt64ToUint64(values[fieldIdxFBTotal].Int64()) * mibToBytes
			metric.MemoryTotal = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxFBTotal].name)
		}
	}
	if fieldIdxGPUUtil < len(values) {
		if isValidInt64FieldValue(values[fieldIdxGPUUtil]) {
			v := float64(values[fieldIdxGPUUtil].Int64())
			metric.GPUUtilization = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxGPUUtil].name)
		}
	}
	if fieldIdxMemUtil < len(values) {
		if isValidFloat64FieldValue(values[fieldIdxMemUtil]) {
			v := values[fieldIdxMemUtil].Float64() * 100
			metric.MemoryUtilization = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxMemUtil].name)
		}
	}
	if fieldIdxFBUsed < len(values) {
		if isValidInt64FieldValue(values[fieldIdxFBUsed]) {
			v := safeInt64ToUint64(values[fieldIdxFBUsed].Int64()) * mibToBytes
			metric.MemoryUsed = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxFBUsed].name)
		}
	}
	if fieldIdxPower < len(values) {
		if isValidFloat64FieldValue(values[fieldIdxPower]) {
			v := values[fieldIdxPower].Float64()
			metric.PowerDraw = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxPower].name)
		}
	}
	if fieldIdxTemp < len(values) {
		if isValidInt64FieldValue(values[fieldIdxTemp]) {
			v := float64(values[fieldIdxTemp].Int64())
			metric.Temperature = &v
		} else {
			skipped = append(skipped, metricsFields[fieldIdxTemp].name)
		}
	}

	return skipped
}

// setupMetricsWatches creates a persistent field group for GPU metrics and
// watches them on all GPUs. Called once during initialization.
// On each collection tick, GetMetrics() reads values via GetLatestValuesForFields()
// without creating or destroying any groups.
func (c *dcgmClient) setupMetricsWatches() {
	c.logger.Info("setting up persistent metrics field watches")

	fieldGroup, err := dcgm.FieldGroupCreate("gpu_metrics_basic", metricsFieldIDs())
	if err != nil {
		c.logger.Error("failed to create metrics field group", zap.Error(err))
		return
	}

	err = dcgm.WatchFieldsWithGroup(fieldGroup, dcgm.GroupAllGPUs())
	if err != nil {
		c.logger.Error("failed to watch metrics fields", zap.Error(err))
		if destroyErr := c.fieldGroupDestroyFunc(fieldGroup); destroyErr != nil {
			c.logger.Debug("failed to destroy unused metrics field group", zap.Error(destroyErr))
		}
		return
	}

	c.metricsFieldGroup = fieldGroup
	c.metricsWatchActive = true
	c.logger.Info("persistent metrics field watches enabled successfully",
		zap.Int("fieldCount", len(metricsFields)))
}

// setupXidWatch creates a persistent field group for DCGM_FI_DEV_XID_ERRORS and
// watches it on all GPUs. Called once during initialization, after setupMetricsWatches.
// If setup fails, xidWatchActive remains false and XID counts default to zero.
func (c *dcgmClient) setupXidWatch() {
	c.logger.Info("setting up XID error field watch")

	fieldGroup, err := dcgm.FieldGroupCreate("gpu_xid_errors", []dcgm.Short{dcgm.DCGM_FI_DEV_XID_ERRORS})
	if err != nil {
		c.logger.Error("failed to create XID field group, XID counting disabled", zap.Error(err))
		return
	}

	err = dcgm.WatchFieldsWithGroup(fieldGroup, dcgm.GroupAllGPUs())
	if err != nil {
		c.logger.Error("failed to watch XID fields, XID counting disabled", zap.Error(err))
		if destroyErr := c.fieldGroupDestroyFunc(fieldGroup); destroyErr != nil {
			c.logger.Debug("failed to destroy unused XID field group", zap.Error(destroyErr))
		}
		return
	}

	c.xidFieldGroup = fieldGroup
	c.xidWatchActive = true
	c.lastXidQueryTime = time.Now()
	c.logger.Info("XID error field watch enabled successfully")
}

// countRestartAppXidsByDevice takes a slice of FieldValue_v2 entries (from GetValuesSince)
// and returns a map of device index to RESTART_APP XID count. Only entries whose Int64()
// value is present in restartAppXIDCodes are counted.
func countRestartAppXidsByDevice(entries []dcgm.FieldValue_v2) map[uint]int64 {
	counts := make(map[uint]int64)
	for _, entry := range entries {
		xidCode := entry.Int64()
		if xidCode < 0 {
			continue
		}
		if restartAppXIDCodes[uint64(xidCode)] {
			counts[entry.EntityID]++
		}
	}
	return counts
}

// mapDeviceIndexToUUID converts a device-index-keyed count map to a UUID-keyed count map.
// Device indices not present in the UUID map are skipped.
func mapDeviceIndexToUUID(countsByDevice map[uint]int64, deviceToUUID map[uint]string) map[string]int64 {
	result := make(map[string]int64)
	for deviceIdx, count := range countsByDevice {
		uuid, ok := deviceToUUID[deviceIdx]
		if !ok {
			continue
		}
		result[uuid] = count
	}
	return result
}

// isValidInt64Value checks if a DCGM int64 value is valid (not a sentinel/blank value).
// DCGM uses specific sentinel values to indicate unavailable or blank data.
func isValidInt64Value(v int64) bool {
	// DCGM sentinel values from dcgm_structs.h. Each width (32-bit and 64-bit)
	// defines four consecutive sentinels starting at the BLANK value:
	//   BLANK          — field exists but no sample collected yet.
	//   NOT_FOUND      — field ID not recognised by this DCGM version.
	//   NOT_SUPPORTED  — field not supported on this GPU/driver.
	//   NOT_PERMISSIONED — insufficient permissions to read this field.
	const (
		int32Blank           = 0x7ffffff0
		int32NotFound        = 0x7ffffff1
		int32NotSupported    = 0x7ffffff2
		int32NotPermissioned = 0x7ffffff3

		int64Blank           = 0x7ffffffffffffff0
		int64NotFound        = 0x7ffffffffffffff1
		int64NotSupported    = 0x7ffffffffffffff2
		int64NotPermissioned = 0x7ffffffffffffff3
	)
	return v != int32Blank && v != int32NotFound && v != int32NotSupported && v != int32NotPermissioned &&
		v != int64Blank && v != int64NotFound && v != int64NotSupported && v != int64NotPermissioned
}

// isValidFloat64Value checks if a DCGM float64 value is valid (not a sentinel/blank value).
func isValidFloat64Value(v float64) bool {
	// DCGM FP64 sentinel values from dcgm_structs.h:
	//   DCGM_FP64_BLANK            = 140737488355328.0
	//   DCGM_FP64_NOT_FOUND       = 140737488355329.0
	//   DCGM_FP64_NOT_SUPPORTED   = 140737488355330.0
	//   DCGM_FP64_NOT_PERMISSIONED = 140737488355331.0
	const (
		fp64Blank           = 140737488355328.0
		fp64NotFound        = 140737488355329.0
		fp64NotSupported    = 140737488355330.0
		fp64NotPermissioned = 140737488355331.0
	)
	return v != fp64Blank && v != fp64NotFound && v != fp64NotSupported && v != fp64NotPermissioned &&
		v >= 0
}

// isValidInt64FieldValue checks if a DCGM FieldValue_v1 contains a valid int64 value.
func isValidInt64FieldValue(fv dcgm.FieldValue_v1) bool {
	return fv.Status == 0 && isValidInt64Value(fv.Int64())
}

// isValidFloat64FieldValue checks if a DCGM FieldValue_v1 contains a valid float64 value.
func isValidFloat64FieldValue(fv dcgm.FieldValue_v1) bool {
	return fv.Status == 0 && isValidFloat64Value(fv.Float64())
}

// safeInt64ToUint64 converts an int64 to uint64, clamping negative values to 0.
// DCGM memory values are always non-negative, but this prevents overflow from sentinel values
// that pass validation.
func safeInt64ToUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v) // #nosec G115 -- negative values are clamped above.
}

// extractXIDCode extracts the XID error code from a policy violation.
// The XID code is stored in the Data field as an XidPolicyCondition.
func extractXIDCode(violation dcgm.PolicyViolation) uint64 {
	if xidData, ok := violation.Data.(dcgm.XidPolicyCondition); ok {
		return uint64(xidData.ErrNum)
	}
	return 0
}

// isCriticalViolation determines if a policy violation should mark the instance as unhealthy.
// XID violations are filtered by code - only well-known critical XID codes cause the instance to be considered unhealthy.
// All other policy types (DBE, NVLink, MaxRtPg, Power, Thermal, PCIe) are always critical.
func (c *dcgmClient) isCriticalViolation(violation dcgm.PolicyViolation) bool {
	// XID violations require filtering by code.
	if violation.Condition == dcgm.XidPolicy {
		xidCode := extractXIDCode(violation)
		return wellKnownXIDCodes[xidCode]
	}

	// All other policy types are critical.
	return true
}

// logPolicyViolation logs detailed information about a policy violation.
// The log level is ERROR for critical violations and WARN for non-critical violations.
// All violations include policy type and timestamp.
// Additional fields are logged based on the specific policy type from the Data field.
func (c *dcgmClient) logPolicyViolation(violation dcgm.PolicyViolation) {
	baseFields := []zap.Field{
		zap.String("condition", string(violation.Condition)),
		zap.Time("timestamp", violation.Timestamp),
	}

	switch violation.Condition {
	case dcgm.XidPolicy:
		if xidData, ok := violation.Data.(dcgm.XidPolicyCondition); ok {
			xidCode := uint64(xidData.ErrNum)
			isCritical := wellKnownXIDCodes[xidCode]

			fields := baseFields
			fields = append(fields,
				zap.Uint64("xidCode", xidCode),
				zap.Bool("critical", isCritical),
				zap.String("xidMessage", getXIDMessage(xidCode)),
			)

			if isCritical {
				c.logger.Error("XID policy violation (critical)", fields...)
			} else {
				c.logger.Warn("XID policy violation (non-critical)", fields...)
			}
		}

	default:
		c.logger.Info("Untracked policy violation", baseFields...)
	}
}

// getXIDMessage returns a human-readable message for a given XID error code.
// For well-known XID codes, it returns a descriptive message explaining the error.
// For unknown codes, it returns a generic message.
func getXIDMessage(code uint64) string {
	// Map of XID codes to human-readable messages.
	// Based on NVIDIA XID error documentation.
	messages := map[uint64]string{
		46:  "GPU stopped processing",
		48:  "Double Bit ECC Error",
		54:  "Auxiliary power connector not connected",
		62:  "Internal micro-controller halt",
		64:  "GPU memory remapping failure",
		74:  "NVLink Error",
		79:  "GPU has fallen off the bus",
		95:  "Uncontained memory error",
		109: "Context switch timeout",
		110: "GPU disappeared from the bus",
		136: "GPU memory page retirement limit exceeded",
		140: "Unrecoverable ECC Error",
		142: "GPU memory page retired due to uncorrectable error",
		143: "GPU memory page retired due to correctable error threshold",
		151: "GPU to CPU interconnect error",
		155: "GPU NVLink flit CRC error",
		156: "GPU NVLink lane error",
		158: "GPU InfoROM corrupted",
	}

	if msg, ok := messages[code]; ok {
		return msg
	}
	return "Unknown XID error"
}
