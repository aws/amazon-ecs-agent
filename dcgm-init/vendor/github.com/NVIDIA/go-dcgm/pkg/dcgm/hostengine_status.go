// Package dcgm provides bindings for NVIDIA's Data Center GPU Manager (DCGM)
package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"unsafe"
)

// Status represents the current resource utilization of the DCGM hostengine process
type Status struct {
	// Memory represents the current memory usage of the DCGM hostengine in kilobytes
	Memory int64
	// CPU represents the current CPU utilization of the DCGM hostengine as a percentage (0-100)
	CPU float64
}

func introspect() (engine Status, err error) {
	var memory C.dcgmIntrospectMemory_t
	memory.version = makeVersion1(unsafe.Sizeof(memory))
	waitIfNoData := 1
	result := C.dcgmIntrospectGetHostengineMemoryUsage(handle.handle, &memory, C.int(waitIfNoData))

	if err = errorString(result); err != nil {
		return engine, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	var cpu C.dcgmIntrospectCpuUtil_t

	cpu.version = makeVersion1(unsafe.Sizeof(cpu))
	result = C.dcgmIntrospectGetHostengineCpuUtilization(handle.handle, &cpu, C.int(waitIfNoData))

	if err = errorString(result); err != nil {
		return engine, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	engine = Status{
		Memory: toInt64(memory.bytesUsed) / 1024,
		CPU:    *dblToFloat(cpu.total) * 100,
	}
	return
}
