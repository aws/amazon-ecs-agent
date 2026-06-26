package dcgm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	dcgmInitCounter int
	mux             sync.Mutex
)

// Init starts DCGM in the specified mode
// Mode can be:
// - Embedded: Start hostengine within this process
// - Standalone: Connect to an already running nv-hostengine
// - StartHostengine: Start and connect to nv-hostengine, terminate before exiting
// Returns a cleanup function and any error encountered
func Init(m mode, args ...string) (cleanup func(), err error) {
	mux.Lock()
	defer mux.Unlock()

	if dcgmInitCounter < 0 {
		count := strconv.Itoa(dcgmInitCounter)
		err = fmt.Errorf("shutdown() is called %s times, before init()", count[1:])
	}

	if dcgmInitCounter == 0 {
		err = initDCGM(m, args...)
		if err != nil {
			return nil, err
		}
	}

	dcgmInitCounter += 1

	return func() {
		if shutdownErr := Shutdown(); shutdownErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to shutdown DCGM with error: `%v`", shutdownErr)
		}
	}, err
}

// Shutdown stops DCGM and destroys all connections
// Returns an error if DCGM is not initialized
func Shutdown() (err error) {
	mux.Lock()
	defer mux.Unlock()

	if dcgmInitCounter <= 0 {
		err = errors.New("init() needs to be called before shutdown()")
	}

	if dcgmInitCounter == 1 {
		err = shutdown()
	}

	dcgmInitCounter -= 1

	return
}

// GetAllDeviceCount returns the count of all GPUs in the system
func GetAllDeviceCount() (uint, error) {
	return getAllDeviceCount()
}

// GetEntityGroupEntities returns all entities of the specified group type
func GetEntityGroupEntities(entityGroup Field_Entity_Group) ([]uint, error) {
	return getEntityGroupEntities(entityGroup)
}

// GetSupportedDevices returns a list of DCGM-supported GPU IDs
func GetSupportedDevices() ([]uint, error) {
	return getSupportedDevices()
}

// GetDeviceInfo returns detailed information about the specified GPU
func GetDeviceInfo(gpuID uint) (Device, error) {
	return getDeviceInfo(gpuID)
}

// GetDeviceStatus returns current status information about the specified GPU
func GetDeviceStatus(gpuID uint) (DeviceStatus, error) {
	return latestValuesForDevice(gpuID)
}

// GetDeviceTopology returns the topology (connectivity) information for the specified GPU
func GetDeviceTopology(gpuID uint) ([]P2PLink, error) {
	return getDeviceTopology(gpuID)
}

// WatchPidFields configures DCGM to start recording stats for GPU processes
// Must be called before GetProcessInfo
func WatchPidFields() (GroupHandle, error) {
	return watchPidFields(time.Microsecond*time.Duration(defaultUpdateFreq), time.Second*time.Duration(defaultMaxKeepAge), defaultMaxKeepSamples)
}

// GetProcessInfo returns detailed per-GPU statistics for the specified process
func GetProcessInfo(group GroupHandle, pid uint) ([]ProcessInfo, error) {
	return getProcessInfo(group, pid)
}

// HealthCheckByGpuId performs a health check on the specified GPU
func HealthCheckByGpuId(gpuID uint) (DeviceHealth, error) {
	return healthCheckByGpuId(gpuID)
}

// ListenForPolicyViolations sets up monitoring for the specified policy conditions on all GPUs
// Returns a channel that receives policy violations and any error encountered
func ListenForPolicyViolations(ctx context.Context, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	groupID := GroupAllGPUs()
	return ListenForPolicyViolationsForGroup(ctx, groupID, typ...)
}

// ListenForPolicyViolationsForGroup sets up policy monitoring for the specified GPU group
// Returns a channel that receives policy violations and any error encountered
func ListenForPolicyViolationsForGroup(ctx context.Context, group GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	return registerPolicy(ctx, group, typ...)
}

// Introspect returns memory and CPU usage statistics for the DCGM hostengine
func Introspect() (Status, error) {
	return introspect()
}

// GetSupportedMetricGroups returns all supported metric groups for the specified GPU
func GetSupportedMetricGroups(gpuID uint) ([]MetricGroup, error) {
	return getSupportedMetricGroups(gpuID)
}

// GetNvLinkLinkStatus returns the status of all NVLink connections
func GetNvLinkLinkStatus() ([]NvLinkStatus, error) {
	return getNvLinkLinkStatus()
}

// GetNvLinkP2PStatus returns the status of NvLinks between GPU pairs
func GetNvLinkP2PStatus() (NvLinkP2PStatus, error) {
	return getNvLinkP2PStatus()
}

// SetPolicyForGroup configures policies with optional custom thresholds and actions for a GPU group
func SetPolicyForGroup(group GroupHandle, configs ...PolicyConfig) error {
	return setPolicyForGroupWithConfig(group, configs...)
}

// GetPolicyForGroup retrieves the current policy configuration for a GPU group
func GetPolicyForGroup(group GroupHandle) (*PolicyStatus, error) {
	return getPolicyForGroup(group)
}

// ClearPolicyForGroup clears all policy conditions for a GPU group
func ClearPolicyForGroup(group GroupHandle) error {
	return clearPolicyForGroup(group)
}

// WatchPolicyViolationsForGroup registers to receive violation notifications for a specific GPU group
func WatchPolicyViolationsForGroup(ctx context.Context, group GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	return registerPolicyOnly(ctx, group, typ...)
}
