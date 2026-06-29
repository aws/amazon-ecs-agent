/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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
#include "dcgm_agent.h"
#include "dcgm_structs.h"
#include "field_values_cb.h"
extern int go_dcgmFieldValueEntityEnumeration(dcgm_field_entity_group_t entityGroupId,
            dcgm_field_eid_t entityId,
            dcgmFieldValue_v1 *values,
            int numValues,
            void *userData);
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

const (
	// maxCallbackValues defines the maximum number of field values that can be
	// accumulated across all callback invocations to prevent unbounded memory growth.
	//
	// DCGM's dcgmGetValuesSince_v2 invokes the callback multiple times (once per entity).
	// The callback receives 'numValues' which varies based on actual data, NOT a fixed 1024.
	// However, to accommodate worst-case scenarios with max entities, max fields, and
	// multiple samples (via maxKeepSamples), we use:
	// 1024 entities × 128 fields × max reasonable samples = 131,072 field values
	//
	// Note: Each FieldValue_v2 is ~32 bytes (not 4KB - C structs aren't kept in Go).
	maxCallbackValues = C.DCGM_GROUP_MAX_ENTITIES_V2 * 128

	// initialCallbackCapacity is the initial capacity for callback value slices.
	// This is a reasonable default that avoids many small reallocations for typical queries.
	initialCallbackCapacity = 256
)

type callback struct {
	mu            sync.Mutex
	Values        []FieldValue_v2
	limitExceeded bool
}

func (cb *callback) processValues(entityGroup Field_Entity_Group, entityID uint, cvalues []C.dcgmFieldValue_v1) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if adding new values would exceed the limit BEFORE conversion
	// to avoid unnecessary allocation and conversion work
	if len(cb.Values)+len(cvalues) > maxCallbackValues {
		// Mark that limit was exceeded so we can return an error
		cb.limitExceeded = true
		return
	}

	// Normal path: convert and append all values
	cb.Values = appendConvertedValues(cb.Values, entityGroup, entityID, cvalues)
}

// appendConvertedValues converts C field values to Go and appends them efficiently.
// This avoids creating an intermediate slice by appending directly.
func appendConvertedValues(dst []FieldValue_v2, entityGroup Field_Entity_Group, entityID uint, cfields []C.dcgmFieldValue_v1) []FieldValue_v2 {
	// Pre-allocate if needed
	if cap(dst)-len(dst) < len(cfields) {
		// Grow the slice capacity efficiently
		newCap := cap(dst) * 2
		if newCap < len(dst)+len(cfields) {
			newCap = len(dst) + len(cfields)
		}
		// If starting from nil/empty, use initialCallbackCapacity as minimum
		if newCap < initialCallbackCapacity {
			newCap = initialCallbackCapacity
		}
		newDst := make([]FieldValue_v2, len(dst), newCap)
		copy(newDst, dst)
		dst = newDst
	}

	// Convert and append directly without intermediate slice
	startLen := len(dst)
	dst = dst[:startLen+len(cfields)]
	for i := range cfields {
		dst[startLen+i] = FieldValue_v2{
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
			dst[startLen+i].StringValue = stringPtr((*C.char)(unsafe.Pointer(&cfields[i].value[0])))
		}
	}

	return dst
}

//export go_dcgmFieldValueEntityEnumeration
func go_dcgmFieldValueEntityEnumeration(
	entityGroup C.dcgm_field_entity_group_t,
	entityID C.dcgm_field_eid_t,
	values *C.dcgmFieldValue_v1,
	numValues C.int,
	userData unsafe.Pointer,
) C.int {
	ptrValues := unsafe.Pointer(values)
	if ptrValues != nil {
		valuesSlice := (*[1 << 30]C.dcgmFieldValue_v1)(ptrValues)[0:numValues]

		if userData != nil {
			processor := (*callback)(userData)
			processor.processValues(Field_Entity_Group(entityGroup), uint(entityID), valuesSlice)
		}
	}
	return 0
}

// GetValuesSince reads and returns field values for a specified group of entities, such as GPUs,
// that have been updated since a given timestamp. It allows for targeted data retrieval based on time criteria.
//
// GPUGroup is a GroupHandle that identifies the group of entities to operate on. It can be obtained from CreateGroup
// for a specific group of GPUs or use GroupAllGPUs() to target all GPUs.
//
// fieldGroup is a FieldHandle representing the group of fields for which data is requested.
//
// sinceTime is a time.Time value representing the timestamp from which to request updated values.
// A zero value (time.Time{}) requests all available data.
//
// Returns []FieldValue_v2 slice containing the requested field values, a time.Time indicating the time
// of the latest data retrieval, and an error if there is any issue during the operation.
//
// If the number of field values exceeds maxCallbackValues (131,072), an error is returned to prevent
// unbounded memory growth. To avoid this, reduce the time range, field group size, or entity count.
func GetValuesSince(gpuGroup GroupHandle, fieldGroup FieldHandle, sinceTime time.Time) ([]FieldValue_v2, time.Time, error) {
	var nextSinceTimestamp C.longlong
	// Start with a nil slice - it will be allocated on first append in the callback.
	// We cannot pre-allocate here due to CGO restrictions on passing Go pointers to C.
	cbResult := &callback{}
	result := C.dcgmGetValuesSince_v2(handle.handle,
		gpuGroup.handle,
		fieldGroup.handle,
		C.longlong(sinceTime.UnixMicro()),
		&nextSinceTimestamp,
		C.dcgmFieldValueEnumeration_f(C.fieldValueEntityCallback),
		unsafe.Pointer(cbResult))
	if result != C.DCGM_ST_OK {
		return nil, time.Time{}, fmt.Errorf("dcgmGetValuesSince_v2 failed with error code %d", int(result))
	}

	if cbResult.limitExceeded {
		return nil, time.Time{}, fmt.Errorf("field value limit exceeded (%d), reduce time range, field count, or entity count", maxCallbackValues)
	}

	return cbResult.Values, timestampUSECToTime(int64(nextSinceTimestamp)), nil
}

func timestampUSECToTime(timestampUSEC int64) time.Time {
	// Convert microseconds to seconds and nanoseconds
	sec := timestampUSEC / 1000000           // Convert microseconds to seconds
	nsec := (timestampUSEC % 1000000) * 1000 // Convert the remaining microseconds to nanoseconds
	// Use time.Unix to get a time.Time object
	return time.Unix(sec, nsec)
}
