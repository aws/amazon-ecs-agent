package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

/*
 *See dcgm_structs.h
 *	DCGM_CPU_CORE_BITMASK_COUNT_V1 (DCGM_MAX_NUM_CPU_CORES / sizeof(uint64_t) / CHAR_BIT)
 *	or
 *	1024 / 8 / 8
 */

const (
	// MAX_NUM_CPU_CORES represents the maximum number of CPU cores supported
	MAX_NUM_CPU_CORES = uint(C.DCGM_MAX_NUM_CPU_CORES)

	// MAX_NUM_CPUS represents the maximum number of CPUs supported
	MAX_NUM_CPUS = uint(C.DCGM_MAX_NUM_CPUS)

	// CHAR_BIT represents the number of bits in a byte
	CHAR_BIT = uint(C.CHAR_BIT)

	// MAX_CPU_CORE_BITMASK_COUNT represents the maximum count of CPU core bitmasks
	MAX_CPU_CORE_BITMASK_COUNT = uint(1024 / 8 / 8)
)

// CPUHierarchyCPU_v1 represents information about a single CPU and its owned cores
type CPUHierarchyCPU_v1 struct {
	// CPUID is the unique identifier for this CPU
	CPUID uint
	// OwnedCores is a bitmask array representing the cores owned by this CPU
	OwnedCores []uint64
}

// CPUHierarchy_v1 represents version 1 of the CPU hierarchy information
type CPUHierarchy_v1 struct {
	// Version is the version number of the hierarchy structure
	Version uint
	// NumCPUs is the number of CPUs in the system
	NumCPUs uint
	// CPUs contains information about each CPU in the system
	CPUs [MAX_NUM_CPUS]CPUHierarchyCPU_v1
}

// GetCPUHierarchy retrieves the CPU hierarchy information from DCGM
func GetCPUHierarchy() (hierarchy CPUHierarchy_v1, err error) {
	var c_hierarchy C.dcgmCpuHierarchy_v1
	c_hierarchy.version = C.dcgmCpuHierarchy_version1
	ptr_hierarchy := (*C.dcgmCpuHierarchy_v1)(unsafe.Pointer(&c_hierarchy))
	result := C.dcgmGetCpuHierarchy(handle.handle, ptr_hierarchy)

	if err = errorString(result); err != nil {
		return toCpuHierarchy(c_hierarchy), fmt.Errorf("error retrieving DCGM CPU hierarchy: %s", err)
	}

	return toCpuHierarchy(c_hierarchy), nil
}

func toCpuHierarchy(c_hierarchy C.dcgmCpuHierarchy_v1) CPUHierarchy_v1 {
	var hierarchy CPUHierarchy_v1
	hierarchy.Version = uint(c_hierarchy.version)
	hierarchy.NumCPUs = uint(c_hierarchy.numCpus)
	for i := uint(0); i < hierarchy.NumCPUs; i++ {
		bits := make([]uint64, MAX_CPU_CORE_BITMASK_COUNT)

		for j := uint(0); j < MAX_CPU_CORE_BITMASK_COUNT; j++ {
			bits[j] = uint64(c_hierarchy.cpus[i].ownedCores.bitmask[j])
		}

		hierarchy.CPUs[i] = CPUHierarchyCPU_v1{
			CPUID:      uint(c_hierarchy.cpus[i].cpuId),
			OwnedCores: bits,
		}
	}

	return hierarchy
}
