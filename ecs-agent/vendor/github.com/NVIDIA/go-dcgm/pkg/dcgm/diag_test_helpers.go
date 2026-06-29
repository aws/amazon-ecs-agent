/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

import (
	"unsafe"
)

// createTestDiagResponse creates a dcgmDiagResponse_v12 for testing
func createTestDiagResponse() C.dcgmDiagResponse_v12 {
	var response C.dcgmDiagResponse_v12
	response.version = C.dcgmDiagResponse_version12
	return response
}

// addInfoMessage adds an info message to a dcgmDiagResponse_v12 for testing
func addInfoMessage(response *C.dcgmDiagResponse_v12, entityID uint, testID uint, message string) {
	idx := response.numInfo
	cStr := C.CString(message)
	defer C.free(unsafe.Pointer(cStr))
	C.strcpy(&response.info[idx].msg[0], cStr)
	response.info[idx].entity.entityId = C.uint(entityID)
	response.info[idx].entity.entityGroupId = C.DCGM_FE_GPU
	response.info[idx].testId = C.uint(testID)
	response.numInfo++
}

// addDiagResult adds a diagnostic result to a dcgmDiagResponse_v12 for testing
func addDiagResult(response *C.dcgmDiagResponse_v12, entityID uint, testID uint, result int) {
	idx := response.numResults
	response.results[idx].entity.entityId = C.uint(entityID)
	response.results[idx].entity.entityGroupId = C.DCGM_FE_GPU
	response.results[idx].testId = C.uint(testID)
	response.results[idx].result = C.dcgmDiagResult_t(result)
	response.numResults++
}

// addEntityWithSerial adds an entity with serial number to a dcgmDiagResponse_v12 for testing
func addEntityWithSerial(response *C.dcgmDiagResponse_v12, entityID uint, serialNumber string) {
	idx := response.numEntities
	cStr := C.CString(serialNumber)
	defer C.free(unsafe.Pointer(cStr))
	C.strcpy(&response.entities[idx].serialNum[0], cStr)
	response.entities[idx].entity.entityId = C.uint(entityID)
	response.entities[idx].entity.entityGroupId = C.DCGM_FE_GPU
	response.numEntities++
}

// Test constants exposed for testing
const (
	testDiagResultPass   = C.DCGM_DIAG_RESULT_PASS
	testDiagResultSkip   = C.DCGM_DIAG_RESULT_SKIP
	testDiagResultWarn   = C.DCGM_DIAG_RESULT_WARN
	testDiagResultFail   = C.DCGM_DIAG_RESULT_FAIL
	testDiagResultNotRun = C.DCGM_DIAG_RESULT_NOT_RUN

	testMemoryIndex          = C.DCGM_MEMORY_INDEX
	testDiagnosticIndex      = C.DCGM_DIAGNOSTIC_INDEX
	testPCIIndex             = C.DCGM_PCI_INDEX
	testSMStressIndex        = C.DCGM_SM_STRESS_INDEX
	testTargetedStressIndex  = C.DCGM_TARGETED_STRESS_INDEX
	testTargetedPowerIndex   = C.DCGM_TARGETED_POWER_INDEX
	testMemoryBandwidthIndex = C.DCGM_MEMORY_BANDWIDTH_INDEX
	testMemtestIndex         = C.DCGM_MEMTEST_INDEX
	testPulseTestIndex       = C.DCGM_PULSE_TEST_INDEX
	testEUDTestIndex         = C.DCGM_EUD_TEST_INDEX
	testSoftwareIndex        = C.DCGM_SOFTWARE_INDEX
	testContextCreateIndex   = C.DCGM_CONTEXT_CREATE_INDEX
)
