package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
	"unsafe"
)

// Time represents a Unix timestamp in seconds
type Time uint64

// String returns a human-readable string representation of the timestamp.
// Returns "Running" if the timestamp is 0, otherwise returns the formatted time.
func (t Time) String() string {
	if t == 0 {
		return "Running"
	}
	tm := time.Unix(int64(t), 0)
	return tm.String()
}

// ProcessUtilInfo contains utilization metrics for a GPU process
type ProcessUtilInfo struct {
	// StartTime is when the process started using the GPU
	StartTime Time
	// EndTime is when the process stopped using the GPU (0 if still running)
	EndTime Time
	// EnergyConsumed is the energy consumed by the process in Joules
	EnergyConsumed *uint64
	// SmUtil is the GPU SM (Streaming Multiprocessor) utilization percentage
	SmUtil *float64
	// MemUtil is the GPU memory utilization percentage
	MemUtil *float64
}

// ViolationTime measures amount of time (in ms) GPU was at reduced clocks
type ViolationTime struct {
	// Power is time spent throttling due to power constraints
	Power *uint64
	// Thermal is time spent throttling due to thermal constraints
	Thermal *uint64
	// Reliability is time spent throttling due to reliability constraints
	Reliability *uint64
	// BoardLimit is time spent throttling due to board limit constraints
	BoardLimit *uint64
	// LowUtilization is time spent throttling due to low utilization
	LowUtilization *uint64
	// SyncBoost is time spent throttling due to sync boost
	SyncBoost *uint64
}

// XIDErrorInfo contains information about XID errors
type XIDErrorInfo struct {
	// NumErrors is the number of XID errors that occurred
	NumErrors int
	// Timestamp contains the timestamps of when XID errors occurred
	Timestamp []uint64
}

// ProcessInfo contains comprehensive information about a GPU process
type ProcessInfo struct {
	// GPU is the ID of the GPU being used
	GPU uint
	// PID is the process ID
	PID uint
	// Name is the name of the process
	Name string
	// ProcessUtilization contains process-specific utilization metrics
	ProcessUtilization ProcessUtilInfo
	// PCI contains PCI bus statistics
	PCI PCIStatusInfo
	// Memory contains memory usage statistics
	Memory MemoryInfo
	// GpuUtilization contains GPU utilization metrics
	GpuUtilization UtilizationInfo
	// Clocks contains GPU clock frequencies
	Clocks ClockInfo
	// Violations contains throttling statistics
	Violations ViolationTime
	// XIDErrors contains XID error information
	XIDErrors XIDErrorInfo
}

// WatchPidFieldsEx is the same as WatchPidFields, but allows for modifying the update frequency, max samples, max
// sample age, and the GPUs on which to enable watches.
func WatchPidFieldsEx(updateFreq, maxKeepAge time.Duration, maxKeepSamples int, gpus ...uint) (GroupHandle, error) {
	return watchPidFields(updateFreq, maxKeepAge, maxKeepSamples, gpus...)
}

func watchPidFields(updateFreq, maxKeepAge time.Duration, maxKeepSamples int, gpus ...uint) (groupId GroupHandle, err error) {
	groupName := fmt.Sprintf("watchPids%d", rand.Uint64())
	group, err := CreateGroup(groupName)
	if err != nil {
		return
	}
	numGpus := len(gpus)

	if numGpus == 0 {
		gpus, err = getSupportedDevices()
		if err != nil {
			return
		}
	}

	for _, gpu := range gpus {
		err = AddToGroup(group, gpu)
		if err != nil {
			return
		}
	}

	result := C.dcgmWatchPidFields(handle.handle, group.handle, C.longlong(updateFreq.Microseconds()), C.double(maxKeepAge.Seconds()), C.int(maxKeepSamples))

	if err = errorString(result); err != nil {
		return groupId, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}
	_ = UpdateAllFields()
	return group, nil
}

func getProcessInfo(groupID GroupHandle, pid uint) (processInfo []ProcessInfo, err error) {
	var pidInfo C.dcgmPidInfo_t
	pidInfo.version = makeVersion2(unsafe.Sizeof(pidInfo))
	pidInfo.pid = C.uint(pid)

	result := C.dcgmGetPidInfo(handle.handle, groupID.handle, &pidInfo)

	if err = errorString(result); err != nil {
		return processInfo, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	name, err := processName(pid)
	if err != nil {
		return processInfo, fmt.Errorf("error getting process name: %s", err)
	}

	processInfo = make([]ProcessInfo, pidInfo.numGpus)

	for i := 0; i < int(pidInfo.numGpus); i++ {
		var energy uint64
		e := *uint64Ptr(pidInfo.gpus[i].energyConsumed)
		if !IsInt64Blank(int64(e)) {
			energy = e / 1000 // mWs to joules
		}

		processUtil := ProcessUtilInfo{
			StartTime:      Time(uint64(pidInfo.gpus[i].startTime) / 1000000),
			EndTime:        Time(uint64(pidInfo.gpus[i].endTime) / 1000000),
			EnergyConsumed: &energy,
			SmUtil:         roundFloat(dblToFloat(pidInfo.gpus[i].processUtilization.smUtil)),
			MemUtil:        roundFloat(dblToFloat(pidInfo.gpus[i].processUtilization.memUtil)),
		}

		// TODO figure out how to deal with blanks
		pci := PCIStatusInfo{
			Throughput: PCIThroughputInfo{
				Rx:      *int64Ptr(pidInfo.gpus[i].pcieRxBandwidth.average),
				Tx:      *int64Ptr(pidInfo.gpus[i].pcieTxBandwidth.average),
				Replays: *int64Ptr(pidInfo.gpus[i].pcieReplays),
			},
		}

		memory := MemoryInfo{
			GlobalUsed: *int64Ptr(pidInfo.gpus[i].maxGpuMemoryUsed), // max gpu memory used for this process
			ECCErrors: ECCErrorsInfo{
				SingleBit: *int64Ptr(C.longlong(pidInfo.gpus[i].eccSingleBit)),
				DoubleBit: *int64Ptr(C.longlong(pidInfo.gpus[i].eccDoubleBit)),
			},
		}

		gpuUtil := UtilizationInfo{
			GPU:    int64(pidInfo.gpus[i].smUtilization.average),
			Memory: int64(pidInfo.gpus[i].memoryUtilization.average),
		}

		violations := ViolationTime{
			Power:          uint64Ptr(pidInfo.gpus[i].powerViolationTime),
			Thermal:        uint64Ptr(pidInfo.gpus[i].thermalViolationTime),
			Reliability:    uint64Ptr(pidInfo.gpus[i].reliabilityViolationTime),
			BoardLimit:     uint64Ptr(pidInfo.gpus[i].boardLimitViolationTime),
			LowUtilization: uint64Ptr(pidInfo.gpus[i].lowUtilizationTime),
			SyncBoost:      uint64Ptr(pidInfo.gpus[i].syncBoostTime),
		}

		clocks := ClockInfo{
			Cores:  *int64Ptr(C.longlong(pidInfo.gpus[i].smClock.average)),
			Memory: *int64Ptr(C.longlong(pidInfo.gpus[i].memoryClock.average)),
		}

		numErrs := int(pidInfo.gpus[i].numXidCriticalErrors)
		ts := make([]uint64, numErrs)
		for j := 0; j < numErrs; j++ {
			ts[j] = uint64(pidInfo.gpus[i].xidCriticalErrorsTs[j])
		}
		xidErrs := XIDErrorInfo{
			NumErrors: numErrs,
			Timestamp: ts,
		}

		processInfo[i].GPU = uint(pidInfo.gpus[i].gpuId)
		processInfo[i].PID = uint(pidInfo.pid)
		processInfo[i].Name = name
		processInfo[i].ProcessUtilization = processUtil
		processInfo[i].PCI = pci
		processInfo[i].Memory = memory
		processInfo[i].GpuUtilization = gpuUtil
		processInfo[i].Clocks = clocks
		processInfo[i].Violations = violations
		processInfo[i].XIDErrors = xidErrs
	}
	return
}

func processName(pid uint) (string, error) {
	f := fmt.Sprintf("/proc/%d/comm", pid)
	b, err := os.ReadFile(f)
	if err != nil {
		// TOCTOU: process terminated
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}
