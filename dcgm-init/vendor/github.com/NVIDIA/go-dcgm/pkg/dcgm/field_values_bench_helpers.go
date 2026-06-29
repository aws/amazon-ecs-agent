package dcgm

// This file contains helpers for benchmarking field value operations.
// These functions expose internal implementation details for performance testing only.
// They should not be used in production code.

/*
#include "dcgm_structs.h"
*/
import "C"
import "unsafe"

// makeTestCFields creates test C field values for benchmarking purposes only.
func makeTestCFields(count int) []C.dcgmFieldValue_v1 {
	cfields := make([]C.dcgmFieldValue_v1, count)
	for i := range cfields {
		cfields[i].fieldId = C.ushort(i)
		cfields[i].fieldType = C.ushort(DCGM_FT_INT64)
		cfields[i].status = C.int(0)
		cfields[i].ts = C.int64_t(1000000 + int64(i))
	}
	return cfields
}

// oldAppendApproach implements the pre-optimization approach for benchmark comparison.
// It creates an intermediate slice before appending, which causes an extra allocation.
func oldAppendApproach(dst []FieldValue_v2, entityGroup Field_Entity_Group, entityID uint, cfields []C.dcgmFieldValue_v1) []FieldValue_v2 {
	intermediate := make([]FieldValue_v2, len(cfields))
	for i := range cfields {
		intermediate[i] = FieldValue_v2{
			Version:       C.dcgmFieldValue_version2,
			EntityGroupId: entityGroup,
			EntityID:      entityID,
			FieldID:       Short(cfields[i].fieldId),
			FieldType:     uint(cfields[i].fieldType),
			Status:        int(cfields[i].status),
			TS:            int64(cfields[i].ts),
			Value:         cfields[i].value,
			StringValue:   nil,
		}
		if uint(cfields[i].fieldType) == DCGM_FT_STRING {
			intermediate[i].StringValue = stringPtr((*C.char)(unsafe.Pointer(&cfields[i].value[0])))
		}
	}
	return append(dst, intermediate...)
}
