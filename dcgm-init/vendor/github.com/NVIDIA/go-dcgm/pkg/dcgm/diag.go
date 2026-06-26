package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"strings"
	"unsafe"
)

// Package dcgm provides bindings for NVIDIA's Data Center GPU Manager (DCGM)

// DIAG_RESULT_STRING_SIZE represents the maximum size of diagnostic result strings
const DIAG_RESULT_STRING_SIZE = 1024

// DiagType represents the type of diagnostic test to run
type DiagType int

const (
	// DiagQuick represents a quick diagnostic test that performs basic health checks
	DiagQuick DiagType = 1

	// DiagMedium represents a medium-length diagnostic test that performs more comprehensive checks
	DiagMedium DiagType = 2

	// DiagLong represents a long diagnostic test that performs extensive health checks
	DiagLong DiagType = 3

	// DiagExtended represents an extended diagnostic test that performs the most thorough system checks
	DiagExtended DiagType = 4
)

// DiagResult represents the result of a single diagnostic test
type DiagResult struct {
	// Status indicates the test result: "pass", "fail", "warn", "skip", or "notrun"
	Status string
	// TestName is the name of the diagnostic test that was run
	TestName string
	// TestOutput contains any additional output or messages from the test
	TestOutput string
	// ErrorCode is the numeric error code if the test failed
	ErrorCode uint
	// ErrorMessage contains a detailed error message if the test failed
	ErrorMessage string
	// Serial number of the tested entity
	SerialNumber string
	// EntityID
	EntityID uint
}

// DiagResults contains the results of all diagnostic tests
type DiagResults struct {
	// Software contains the results of software-related diagnostic tests
	Software []DiagResult
}

// diagResultString converts a diagnostic result code to its string representation
func diagResultString(r int) string {
	switch r {
	case C.DCGM_DIAG_RESULT_PASS:
		return "pass"
	case C.DCGM_DIAG_RESULT_SKIP:
		return "skipped"
	case C.DCGM_DIAG_RESULT_WARN:
		return "warn"
	case C.DCGM_DIAG_RESULT_FAIL:
		return "fail"
	case C.DCGM_DIAG_RESULT_NOT_RUN:
		return "notrun"
	}
	return ""
}

// gpuTestName returns the category name for a diagnostic test based on its test ID.
// This function handles all diagnostic test types including GPU tests and software tests.
// Software tests (DCGM_SWTEST_*) all report under DCGM_SOFTWARE_INDEX and return "software".
// Detailed test information is provided in TestOutput, not in the TestName.
func gpuTestName(t int) string {
	switch t {
	case C.DCGM_MEMORY_INDEX:
		return "memory"
	case C.DCGM_DIAGNOSTIC_INDEX:
		return "diagnostic"
	case C.DCGM_PCI_INDEX:
		return "pcie"
	case C.DCGM_SM_STRESS_INDEX:
		return "sm stress"
	case C.DCGM_TARGETED_STRESS_INDEX:
		return "targeted stress"
	case C.DCGM_TARGETED_POWER_INDEX:
		return "targeted power"
	case C.DCGM_MEMORY_BANDWIDTH_INDEX:
		return "memory bandwidth"
	case C.DCGM_MEMTEST_INDEX:
		return "memtest"
	case C.DCGM_PULSE_TEST_INDEX:
		return "pulse"
	case C.DCGM_EUD_TEST_INDEX:
		return "eud"
	case C.DCGM_SOFTWARE_INDEX:
		return "software"
	case C.DCGM_CONTEXT_CREATE_INDEX:
		return "context create"
	}
	return ""
}

func getErrorMsg(entityId uint, testId uint, response C.dcgmDiagResponse_v12) (msg string, code uint) {
	for i := 0; i < int(response.numErrors); i++ {
		if uint(response.errors[i].entity.entityId) != entityId || uint(response.errors[i].testId) != testId {
			continue
		}

		msg = C.GoString((*C.char)(unsafe.Pointer(&response.errors[i].msg)))
		code = uint(response.errors[i].code)
		return
	}

	return
}

func getInfoMsg(entityId uint, testId uint, response C.dcgmDiagResponse_v12) string {
	var msgs []string
	for i := 0; i < int(response.numInfo); i++ {
		if uint(response.info[i].entity.entityId) != entityId || uint(response.info[i].testId) != testId {
			continue
		}
		msgs = append(msgs, C.GoString((*C.char)(unsafe.Pointer(&response.info[i].msg))))
	}
	return strings.Join(msgs, " | ")
}

func getTestName(resultIdx uint, response C.dcgmDiagResponse_v12) string {
	for i := uint(0); i < uint(response.numTests); i++ {
		t := response.tests[i]
		for j := uint16(0); j < uint16(t.numResults); j++ {
			if uint16(t.resultIndices[j]) == uint16(resultIdx) {
				plugin := C.GoString((*C.char)(unsafe.Pointer(&t.pluginName)))
				if plugin != "" {
					plugin = "/" + plugin
				}
				return C.GoString((*C.char)(unsafe.Pointer(&t.name))) + plugin
			}
		}
	}
	return ""
}

func getSerial(resultIdx uint, response C.dcgmDiagResponse_v12) string {
	for i := 0; i < int(response.numEntities); i++ {
		if response.entities[i].entity.entityId == response.results[resultIdx].entity.entityId &&
			response.entities[i].entity.entityGroupId == response.results[resultIdx].entity.entityGroupId {
			return C.GoString((*C.char)(unsafe.Pointer(&response.entities[i].serialNum)))
		}
	}
	return ""
}

func newDiagResult(resultIndex uint, response C.dcgmDiagResponse_v12) DiagResult {
	entityId := uint(response.results[resultIndex].entity.entityId)
	testId := uint(response.results[resultIndex].testId)

	msg, code := getErrorMsg(entityId, testId, response)
	info := getInfoMsg(entityId, testId, response)
	testName := gpuTestName(int(testId))
	serial := getSerial(resultIndex, response)

	return DiagResult{
		Status:       diagResultString(int(response.results[resultIndex].result)),
		TestName:     testName,
		TestOutput:   info,
		ErrorCode:    code,
		ErrorMessage: msg,
		SerialNumber: serial,
		EntityID:     entityId,
	}
}

func diagLevel(diagType DiagType) C.dcgmDiagnosticLevel_t {
	switch diagType {
	case DiagQuick:
		return C.DCGM_DIAG_LVL_SHORT
	case DiagMedium:
		return C.DCGM_DIAG_LVL_MED
	case DiagLong:
		return C.DCGM_DIAG_LVL_LONG
	case DiagExtended:
		return C.DCGM_DIAG_LVL_XLONG
	}
	return C.DCGM_DIAG_LVL_INVALID
}

// RunDiag runs diagnostic tests on a group of GPUs with the specified diagnostic level.
// Parameters:
//   - diagType: The type/level of diagnostic test to run (Quick, Medium, Long, or Extended)
//   - groupId: The group of GPUs to run diagnostics on
//
// Returns:
//   - DiagResults containing the results of all diagnostic tests
//   - error if the diagnostics failed to run
func RunDiag(diagType DiagType, groupID GroupHandle) (DiagResults, error) {
	var diagResults C.dcgmDiagResponse_v12
	diagResults.version = C.dcgmDiagResponse_version12

	result := C.dcgmRunDiagnostic(handle.handle, groupID.handle, diagLevel(diagType), &diagResults)
	if err := errorString(result); err != nil {
		return DiagResults{}, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	var diagRun DiagResults
	diagRun.Software = make([]DiagResult, diagResults.numResults)
	for i := 0; i < int(diagResults.numResults); i++ {
		diagRun.Software[i] = newDiagResult(uint(i), diagResults)
	}

	return diagRun, nil
}
