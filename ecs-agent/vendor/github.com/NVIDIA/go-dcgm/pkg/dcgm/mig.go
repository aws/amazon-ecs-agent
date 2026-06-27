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

// Field_Entity_Group represents the type of DCGM entity
type Field_Entity_Group uint

const (
	// FE_NONE represents no entity type
	FE_NONE Field_Entity_Group = iota
	// FE_GPU represents a GPU device entity
	FE_GPU
	// FE_VGPU represents a virtual GPU entity
	FE_VGPU
	// FE_SWITCH represents an NVSwitch entity
	FE_SWITCH
	// FE_GPU_I represents a GPU instance entity
	FE_GPU_I
	// FE_GPU_CI represents a GPU compute instance entity
	FE_GPU_CI
	// FE_LINK represents an NVLink entity
	FE_LINK
	// FE_CPU represents a CPU entity
	FE_CPU
	// FE_CPU_CORE represents a CPU core entity
	FE_CPU_CORE
	// FE_COUNT represents the total number of entity types
	FE_COUNT
)

// String returns a string representation of the Field_Entity_Group
func (e Field_Entity_Group) String() string {
	switch e {
	case FE_GPU:
		return "GPU"
	case FE_VGPU:
		return "vGPU"
	case FE_SWITCH:
		return "NvSwitch"
	case FE_GPU_I:
		return "GPU Instance"
	case FE_GPU_CI:
		return "GPU Compute Instance"
	case FE_LINK:
		return "NvLink"
	case FE_CPU:
		return "CPU"
	case FE_CPU_CORE:
		return "CPU Core"
	}
	return "unknown"
}

// GroupEntityPair represents a DCGM entity and its group identifier
type GroupEntityPair struct {
	// EntityGroupId specifies the type of the entity
	EntityGroupId Field_Entity_Group
	// EntityId is the unique identifier for this entity
	EntityId uint
}

// MigEntityInfo contains information about a MIG entity
type MigEntityInfo struct {
	// GpuUuid is the UUID of the parent GPU
	GpuUuid string
	// NvmlGpuIndex is the NVML index of the parent GPU
	NvmlGpuIndex uint
	// NvmlInstanceId is the NVML GPU instance ID
	NvmlInstanceId uint
	// NvmlComputeInstanceId is the NVML compute instance ID
	NvmlComputeInstanceId uint
	// NvmlMigProfileId is the NVML MIG profile ID
	NvmlMigProfileId uint
	// NvmlProfileSlices is the number of slices in the MIG profile
	NvmlProfileSlices uint
}

// MigHierarchyInfo_v2 represents version 2 of MIG hierarchy information
type MigHierarchyInfo_v2 struct {
	// Entity contains the entity information
	Entity GroupEntityPair
	// Parent contains the parent entity information
	Parent GroupEntityPair
	// Info contains detailed MIG entity information
	Info MigEntityInfo
}

const (
	// MAX_NUM_DEVICES represents the maximum number of GPU devices supported
	MAX_NUM_DEVICES = uint(C.DCGM_MAX_NUM_DEVICES)

	// MAX_HIERARCHY_INFO represents the maximum size of the MIG hierarchy information
	MAX_HIERARCHY_INFO = uint(C.DCGM_MAX_HIERARCHY_INFO)
)

// MigHierarchy_v2 represents version 2 of the complete MIG hierarchy
type MigHierarchy_v2 struct {
	// Version is the version number of the hierarchy structure
	Version uint
	// Count is the number of valid entries in EntityList
	Count uint
	// EntityList contains the MIG hierarchy information for each entity
	EntityList [C.DCGM_MAX_HIERARCHY_INFO]MigHierarchyInfo_v2
}

// GetGPUInstanceHierarchy retrieves the complete MIG hierarchy information
func GetGPUInstanceHierarchy() (hierarchy MigHierarchy_v2, err error) {
	var c_hierarchy C.dcgmMigHierarchy_v2
	c_hierarchy.version = C.dcgmMigHierarchy_version2
	ptr_hierarchy := (*C.dcgmMigHierarchy_v2)(unsafe.Pointer(&c_hierarchy))
	result := C.dcgmGetGpuInstanceHierarchy(handle.handle, ptr_hierarchy)

	if err = errorString(result); err != nil {
		return toMigHierarchy(c_hierarchy), fmt.Errorf("error retrieving DCGM MIG hierarchy: %s", err)
	}

	return toMigHierarchy(c_hierarchy), nil
}

func toMigHierarchy(c_hierarchy C.dcgmMigHierarchy_v2) MigHierarchy_v2 {
	var hierarchy MigHierarchy_v2
	hierarchy.Version = uint(c_hierarchy.version)
	hierarchy.Count = uint(c_hierarchy.count)
	for i := uint(0); i < hierarchy.Count; i++ {
		hierarchy.EntityList[i] = MigHierarchyInfo_v2{
			Entity: GroupEntityPair{Field_Entity_Group(c_hierarchy.entityList[i].entity.entityGroupId), uint(c_hierarchy.entityList[i].entity.entityId)},
			Parent: GroupEntityPair{Field_Entity_Group(c_hierarchy.entityList[i].parent.entityGroupId), uint(c_hierarchy.entityList[i].parent.entityId)},
			Info: MigEntityInfo{
				GpuUuid:               *stringPtr(&c_hierarchy.entityList[i].info.gpuUuid[0]),
				NvmlGpuIndex:          uint(c_hierarchy.entityList[i].info.nvmlGpuIndex),
				NvmlInstanceId:        uint(c_hierarchy.entityList[i].info.nvmlInstanceId),
				NvmlComputeInstanceId: uint(c_hierarchy.entityList[i].info.nvmlComputeInstanceId),
				NvmlMigProfileId:      uint(c_hierarchy.entityList[i].info.nvmlMigProfileId),
				NvmlProfileSlices:     uint(c_hierarchy.entityList[i].info.nvmlProfileSlices),
			},
		}
	}

	return hierarchy
}
