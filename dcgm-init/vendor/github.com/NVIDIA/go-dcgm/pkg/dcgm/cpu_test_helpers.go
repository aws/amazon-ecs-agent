/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dcgm

/*
#include <stdlib.h>
#include <string.h>
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import "unsafe"

const (
	testCPUHierarchyVersion2       = C.dcgmCpuHierarchy_version2
	testDCGMStatusVersionMismatch  = C.DCGM_ST_VER_MISMATCH
	testDCGMStatusFunctionNotFound = C.DCGM_ST_FUNCTION_NOT_FOUND
)

type testDCGMReturn = C.dcgmReturn_t

type testCPUHierarchyV2CPU struct {
	cpuID             uint
	ownedCoreBitmasks map[int]uint64
	serial            string
}

// createTestCPUHierarchyV2 creates and converts a dcgmCpuHierarchy_v2 for testing.
func createTestCPUHierarchyV2(cpus []testCPUHierarchyV2CPU) CPUHierarchy_v2 {
	var hierarchy C.dcgmCpuHierarchy_v2
	hierarchy.version = C.dcgmCpuHierarchy_version2
	hierarchy.numCpus = C.uint(len(cpus))

	for i, cpu := range cpus {
		hierarchy.cpus[i].cpuId = C.uint(cpu.cpuID)

		for bitmaskIndex, bitmask := range cpu.ownedCoreBitmasks {
			hierarchy.cpus[i].ownedCores.bitmask[bitmaskIndex] = C.uint64_t(bitmask)
		}

		cSerial := C.CString(cpu.serial)
		C.strncpy(&hierarchy.cpus[i].serial[0], cSerial, C.DCGM_MAX_STR_LENGTH-1)
		C.free(unsafe.Pointer(cSerial))
	}

	return toCpuHierarchy_v2(hierarchy)
}
