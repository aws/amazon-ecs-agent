package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"unsafe"
)

// MetricGroup represents a group of metrics for a specific GPU
type MetricGroup struct {
	Major    uint
	Minor    uint
	FieldIds []uint
}

func getSupportedMetricGroups(gpuID uint) ([]MetricGroup, error) {
	var (
		groupInfo C.dcgmProfGetMetricGroups_t
		err       error
		groups    []MetricGroup
	)

	groupInfo.version = makeVersion3(unsafe.Sizeof(groupInfo))

	groupInfo.gpuId = C.uint(gpuID)

	result := C.dcgmProfGetSupportedMetricGroups(handle.handle, &groupInfo)

	if err = errorString(result); err != nil {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	count := uint(groupInfo.numMetricGroups)

	groups = make([]MetricGroup, count)
	for i := uint(0); i < count; i++ {
		groups[i].Major = uint(groupInfo.metricGroups[i].majorId)
		groups[i].Minor = uint(groupInfo.metricGroups[i].minorId)

		fieldCount := uint(groupInfo.metricGroups[i].numFieldIds)

		groups[i].FieldIds = make([]uint, fieldCount)
		for j := uint(0); j < fieldCount; j++ {
			groups[i].FieldIds[j] = uint(groupInfo.metricGroups[i].fieldIds[j])
		}
	}

	return groups, nil
}
