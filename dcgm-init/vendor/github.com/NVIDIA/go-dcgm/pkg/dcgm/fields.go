package dcgm

//go:generate go run ../../cmd/gen-fields/main.go ../../cmd/gen-fields/template.go --legacy-fields legacy_fields.csv dcgm_fields.h const_fields.go

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"sync"
	"unicode"
	"unsafe"
)

const (
	// defaultUpdateFreq specifies the default update frequency in microseconds
	defaultUpdateFreq = 30000000 // usec

	// defaultMaxKeepAge specifies the default maximum age to keep samples in seconds
	defaultMaxKeepAge = 0 // sec

	// defaultMaxKeepSamples specifies the default number of samples to keep
	defaultMaxKeepSamples = 1

	// fieldValuesSliceSize is the initial capacity for pooled field value slices.
	// This is kept small to avoid wasting memory when only a few fields are needed.
	// Note: Each C.dcgmFieldValue_v1 struct is ~4KB (due to 4096-byte value array),
	// so even small allocations are significant:
	//   - 2 fields = ~8 KB
	//   - 32 fields = ~128 KB
	//   - 128 fields (max) = ~512 KB
	// This is a fundamental limitation of DCGM's C API which requires pre-allocated arrays.
	fieldValuesSliceSize = 32

	// poolCapacityThreshold defines the threshold above which we don't use the pool.
	// For very large requests, it's better to allocate directly rather than grow pool slices.
	// This is set at 2x DCGM_MAX_FIELD_IDS_PER_FIELD_GROUP to accommodate typical use cases.
	// Beyond this threshold (~1 MB per allocation), we bypass the pool entirely.
	poolCapacityThreshold = 256
)

// FieldMeta represents metadata about a DCGM field, including its identifier,
// type, size, and other attributes. This struct is used to describe the
// characteristics and properties of fields that can be monitored or queried
// through DCGM.
type FieldMeta struct {
	FieldID     Short              // Unique identifier for the field
	FieldType   byte               // Type of the field (e.g., integer, float, string)
	Size        byte               // Size of the field in bytes
	Tag         string             // Human-readable tag/name for the field
	Scope       int                // Scope of the field
	NvmlFieldID int                // Corresponding NVML field identifier
	EntityLevel Field_Entity_Group // Entity level/group this field belongs to
}

// FieldHandle represents a handle to a DCGM field group
type FieldHandle struct{ handle C.dcgmFieldGrp_t }

// SetHandle sets the internal DCGM field group handle to the provided value
func (f *FieldHandle) SetHandle(val uintptr) {
	f.handle = C.dcgmGpuGrp_t(val)
}

// GetHandle returns the internal DCGM field group handle as a uintptr
func (f *FieldHandle) GetHandle() uintptr {
	return uintptr(f.handle)
}

// FieldGroupCreate creates a new field group with the specified fields.
// fieldsGroupName is the name for the new group.
// fields is a slice of field IDs to include in the group.
// Returns the field group handle and any error encountered.
//
// Important: Field groups must be destroyed using FieldGroupDestroy when no longer
// needed to prevent resource leaks in the DCGM library.
//
// Example:
//
//	fieldGroup, err := dcgm.FieldGroupCreate("myFields", []dcgm.Short{dcgm.DCGM_FI_DEV_POWER_USAGE})
//	if err != nil {
//	    return err
//	}
//	defer dcgm.FieldGroupDestroy(fieldGroup)
//
//	// Use the field group...
func FieldGroupCreate(fieldsGroupName string, fields []Short) (fieldsId FieldHandle, err error) {
	var fieldsGroup C.dcgmFieldGrp_t
	cfields := make([]C.ushort, len(fields))
	for i, f := range fields {
		cfields[i] = C.ushort(f)
	}

	groupName := C.CString(fieldsGroupName)
	defer freeCString(groupName)

	result := C.dcgmFieldGroupCreate(handle.handle, C.int(len(fields)), &cfields[0], groupName, &fieldsGroup)
	if err = errorString(result); err != nil {
		return fieldsId, fmt.Errorf("error creating DCGM fields group: %s", err)
	}

	fieldsId = FieldHandle{fieldsGroup}
	return fieldsId, err
}

// FieldGroupDestroy destroys a previously created field group.
// Returns an error if the group cannot be destroyed.
func FieldGroupDestroy(fieldsGroup FieldHandle) (err error) {
	result := C.dcgmFieldGroupDestroy(handle.handle, fieldsGroup.handle)
	if err = errorString(result); err != nil {
		err = fmt.Errorf("error destroying DCGM fields group: %s", err)
	}

	return err
}

// WatchFields starts monitoring the specified fields for a GPU.
// gpuId is the ID of the GPU to monitor.
// fieldsGroup is the handle of the field group to watch.
// groupName is a name for the watch group.
// Returns a group handle and any error encountered.
func WatchFields(gpuID uint, fieldsGroup FieldHandle, groupName string) (groupId GroupHandle, err error) {
	return watchFieldsWithUpdater(UpdateAllFields, gpuID, fieldsGroup, groupName)
}

func watchFieldsWithUpdater(update func() error, gpuID uint, fieldsGroup FieldHandle, groupName string) (groupId GroupHandle, err error) {
	group, err := CreateGroup(groupName)
	if err != nil {
		return groupId, err
	}
	defer func() {
		if err != nil {
			_ = DestroyGroup(group)
		}
	}()

	err = AddToGroup(group, gpuID)
	if err != nil {
		return groupId, err
	}

	result := C.dcgmWatchFields(handle.handle, group.handle, fieldsGroup.handle, C.longlong(defaultUpdateFreq),
		C.double(defaultMaxKeepAge), C.int(defaultMaxKeepSamples))
	if err = errorString(result); err != nil {
		return groupId, fmt.Errorf("error watching fields: %s", err)
	}

	if err = update(); err != nil {
		return groupId, err
	}
	return group, nil
}

// WatchFieldsWithGroupEx starts monitoring fields with custom parameters.
// fieldsGroup is the handle of the field group to watch.
// group is the group handle to associate with the watch.
// updateFreq is the update frequency in microseconds.
// maxKeepAge is the maximum age of samples to keep in seconds.
// maxKeepSamples is the maximum number of samples to keep.
// Returns an error if the watch operation fails.
func WatchFieldsWithGroupEx(
	fieldsGroup FieldHandle, group GroupHandle, updateFreq int64, maxKeepAge float64, maxKeepSamples int32,
) error {
	result := C.dcgmWatchFields(handle.handle, group.handle, fieldsGroup.handle,
		C.longlong(updateFreq), C.double(maxKeepAge), C.int(maxKeepSamples))

	if err := errorString(result); err != nil {
		return fmt.Errorf("error watching fields: %s", err)
	}

	if err := UpdateAllFields(); err != nil {
		return err
	}

	return nil
}

// WatchFieldsWithGroup starts monitoring fields using default parameters.
// fieldsGroup is the handle of the field group to watch.
// group is the group handle to associate with the watch.
// Returns an error if the watch operation fails.
func WatchFieldsWithGroup(fieldsGroup FieldHandle, group GroupHandle) error {
	return WatchFieldsWithGroupEx(fieldsGroup, group, defaultUpdateFreq, defaultMaxKeepAge, defaultMaxKeepSamples)
}

// UnwatchFields stops monitoring the specified fields for a GPU group.
// fieldsGroup is the handle to the field group to stop watching.
// group is the handle to the GPU group to stop watching.
func UnwatchFields(fieldsGroup FieldHandle, group GroupHandle) error {
	result := C.dcgmUnwatchFields(handle.handle, group.handle, fieldsGroup.handle)
	if err := errorString(result); err != nil {
		return fmt.Errorf("error unwatching fields: %w", err)
	}
	return nil
}

var fieldValuePool = sync.Pool{
	New: func() any {
		slice := make([]C.dcgmFieldValue_v1, 0, fieldValuesSliceSize)
		return &slice
	},
}

var fieldValueV2Pool = sync.Pool{
	New: func() any {
		slice := make([]C.dcgmFieldValue_v2, 0, fieldValuesSliceSize)
		return &slice
	},
}

func acquireSlice[T any](pool *sync.Pool, size int) []T {
	if v := pool.Get(); v != nil {
		if slice, ok := v.(*[]T); ok && cap(*slice) >= size {
			s := *slice
			return s[:size]
		}
		// Return mismatched type back to pool to avoid polluting it
		pool.Put(v)
	}
	return make([]T, size)
}

func releaseSlice[T any](pool *sync.Pool, slice []T) {
	// Clear the slice to release references to elements
	clear(slice)
	slice = slice[:0]
	pool.Put(&slice)
}

func acquireFieldValueSlice(size int) []C.dcgmFieldValue_v1 {
	// For very large requests, don't use the pool to avoid keeping huge slices around.
	// Note: Each dcgmFieldValue_v1 is ~4KB, so 256 elements = ~1MB.
	// Beyond this threshold, we allocate directly and let GC handle cleanup.
	if size > poolCapacityThreshold {
		return make([]C.dcgmFieldValue_v1, size)
	}

	if v := fieldValuePool.Get(); v != nil {
		if slice, ok := v.(*[]C.dcgmFieldValue_v1); ok {
			s := *slice
			// If the pooled slice is much larger than needed, don't use it
			// to avoid keeping oversized slices in memory.
			// We allow up to 4x the requested size to avoid excessive allocation churn,
			// but beyond that we prefer a fresh allocation to avoid memory bloat.
			if cap(s) >= size && cap(s) <= size*4 {
				return s[:size]
			}
			// Return oversized slice back to pool for potential later reuse
			fieldValuePool.Put(v)
		} else {
			fieldValuePool.Put(v)
		}
	}
	return make([]C.dcgmFieldValue_v1, size)
}

func releaseFieldValueSlice(slice []C.dcgmFieldValue_v1) {
	// Don't return very large slices to the pool
	if cap(slice) > poolCapacityThreshold {
		return
	}
	clear(slice)
	slice = slice[:0]
	fieldValuePool.Put(&slice)
}

func acquireFieldValueV2Slice(size int) []C.dcgmFieldValue_v2 {
	// For very large requests, don't use the pool to avoid keeping huge slices around.
	// Note: Each dcgmFieldValue_v2 is also ~4KB+ due to the value array.
	// Beyond poolCapacityThreshold, we allocate directly and let GC handle cleanup.
	if size > poolCapacityThreshold {
		return make([]C.dcgmFieldValue_v2, size)
	}

	if v := fieldValueV2Pool.Get(); v != nil {
		if slice, ok := v.(*[]C.dcgmFieldValue_v2); ok {
			s := *slice
			// If the pooled slice is much larger than needed, don't use it
			// to avoid keeping oversized slices in memory.
			// We allow up to 4x the requested size to balance memory usage vs allocation overhead.
			if cap(s) >= size && cap(s) <= size*4 {
				return s[:size]
			}
			// Return oversized slice back to pool for potential later reuse
			fieldValueV2Pool.Put(v)
		} else {
			fieldValueV2Pool.Put(v)
		}
	}
	return make([]C.dcgmFieldValue_v2, size)
}

func releaseFieldValueV2Slice(slice []C.dcgmFieldValue_v2) {
	// Don't return very large slices to the pool
	if cap(slice) > poolCapacityThreshold {
		return
	}
	clear(slice)
	slice = slice[:0]
	fieldValueV2Pool.Put(&slice)
}

// GetLatestValuesForFields retrieves the most recent values for the specified fields.
// gpu is the ID of the GPU to query.
// fields is a slice of field IDs to retrieve.
// Returns a slice of field values and any error encountered.
func GetLatestValuesForFields(gpu uint, fields []Short) ([]FieldValue_v1, error) {
	values := acquireFieldValueSlice(len(fields))
	defer releaseFieldValueSlice(values)

	cfields := make([]C.ushort, len(fields))
	for i, f := range fields {
		cfields[i] = C.ushort(f)
	}

	result := C.dcgmGetLatestValuesForFields(handle.handle, C.int(gpu), &cfields[0], C.uint(len(fields)), &values[0])
	if err := errorString(result); err != nil {
		return nil, fmt.Errorf("error watching fields: %s", err)
	}

	// Convert to our return type before returning
	return toFieldValue(values), nil
}

// LinkGetLatestValues retrieves the latest values for specified fields of a link entity.
// index is the link index.
// parentId is the ID of the parent entity.
// fields is a slice of field IDs to retrieve.
// Returns a slice of field values and any error encountered.
func LinkGetLatestValues(index uint, parentType Field_Entity_Group, parentId uint, fields []Short) ([]FieldValue_v1, error) {
	slice := make([]byte, 4)
	slice[0] = uint8(parentType)
	binary.LittleEndian.PutUint16(slice[1:3], uint16(index))
	slice[3] = uint8(parentId)
	entityId := binary.LittleEndian.Uint32(slice)
	return EntityGetLatestValues(FE_LINK, uint(entityId), fields)
}

// EntityGetLatestValues retrieves the latest values for specified fields of any entity.
// entityGroup specifies the type of entity to query.
// entityId is the ID of the entity.
// fields is a slice of field IDs to retrieve.
// Returns a slice of field values and any error encountered.
func EntityGetLatestValues(entityGroup Field_Entity_Group, entityId uint, fields []Short) ([]FieldValue_v1, error) {
	values := acquireFieldValueSlice(len(fields))
	defer releaseFieldValueSlice(values)

	cfields := make([]C.ushort, len(fields))
	for i, f := range fields {
		cfields[i] = C.ushort(f)
	}

	result := C.dcgmEntityGetLatestValues(handle.handle, C.dcgm_field_entity_group_t(entityGroup), C.int(entityId),
		&cfields[0], C.uint(len(fields)), &values[0])
	if result != C.DCGM_ST_OK {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	return toFieldValue(values), nil
}

// EntitiesGetLatestValues retrieves the latest values for specified fields across multiple entities.
// entities is a slice of entity pairs to query.
// fields is a slice of field IDs to retrieve.
// flags specify additional options for the query.
// Returns a slice of field values and any error encountered.
func EntitiesGetLatestValues(entities []GroupEntityPair, fields []Short, flags uint) ([]FieldValue_v2, error) {
	values := acquireFieldValueV2Slice(len(fields) * len(entities))
	defer releaseFieldValueV2Slice(values)

	cfields := make([]C.ushort, len(fields))
	for i, f := range fields {
		cfields[i] = C.ushort(f)
	}
	cEntities := make([]C.dcgmGroupEntityPair_t, len(entities))
	cPtrEntities := *(*[]C.dcgmGroupEntityPair_t)(unsafe.Pointer(&cEntities))
	for i, entity := range entities {
		cEntities[i] = C.dcgmGroupEntityPair_t{
			C.dcgm_field_entity_group_t(entity.EntityGroupId),
			C.dcgm_field_eid_t(entity.EntityId),
		}
	}

	result := C.dcgmEntitiesGetLatestValues(handle.handle, &cPtrEntities[0], C.uint(len(entities)), &cfields[0],
		C.uint(len(fields)), C.uint(flags), &values[0])
	if err := errorString(result); err != nil {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	return toFieldValue_v2(values), nil
}

// UpdateAllFields forces an update of all field values.
// Returns an error if the update fails.
func UpdateAllFields() error {
	waitForUpdate := C.int(1)
	result := C.dcgmUpdateAllFields(handle.handle, waitForUpdate)

	return errorString(result)
}

func toFieldValue(cfields []C.dcgmFieldValue_v1) []FieldValue_v1 {
	fields := make([]FieldValue_v1, len(cfields))
	for i := range cfields {
		fields[i] = FieldValue_v1{
			Version:   uint(cfields[i].version),
			FieldID:   Short(cfields[i].fieldId),
			FieldType: uint(cfields[i].fieldType),
			Status:    int(cfields[i].status),
			TS:        int64(cfields[i].ts),
			Value:     cfields[i].value,
		}
	}

	return fields
}

// Int64 returns the field value as an int64.
func (fv FieldValue_v1) Int64() int64 {
	return *(*int64)(unsafe.Pointer(&fv.Value[0]))
}

// Float64 returns the field value as a float64.
func (fv FieldValue_v1) Float64() float64 {
	return *(*float64)(unsafe.Pointer(&fv.Value[0]))
}

// String returns the field value as a string.
func (fv FieldValue_v1) String() string {
	return C.GoString((*C.char)(unsafe.Pointer(&fv.Value[0])))
}

// Blob returns the raw field value as a byte array.
func (fv FieldValue_v1) Blob() [4096]byte {
	return fv.Value
}

func toFieldValue_v2(cfields []C.dcgmFieldValue_v2) []FieldValue_v2 {
	fields := make([]FieldValue_v2, len(cfields))
	for i := range cfields {
		if uint(cfields[i].fieldType) == DCGM_FT_STRING {
			fields[i] = FieldValue_v2{
				Version:       uint(cfields[i].version),
				EntityGroupId: Field_Entity_Group(cfields[i].entityGroupId),
				EntityID:      uint(cfields[i].entityId),
				FieldID:       Short(cfields[i].fieldId),
				FieldType:     uint(cfields[i].fieldType),
				Status:        int(cfields[i].status),
				TS:            int64(cfields[i].ts),
				Value:         cfields[i].value,
				StringValue:   stringPtr((*C.char)(unsafe.Pointer(&cfields[i].value[0]))),
			}
		} else {
			fields[i] = FieldValue_v2{
				Version:       uint(cfields[i].version),
				EntityGroupId: Field_Entity_Group(cfields[i].entityGroupId),
				EntityID:      uint(cfields[i].entityId),
				FieldID:       Short(cfields[i].fieldId),
				FieldType:     uint(cfields[i].fieldType),
				Status:        int(cfields[i].status),
				TS:            int64(cfields[i].ts),
				Value:         cfields[i].value,
				StringValue:   nil,
			}
		}
	}

	return fields
}

func dcgmFieldValue_v1ToFieldValue_v2(
	fieldEntityGroup Field_Entity_Group, entityId uint, cfields []C.dcgmFieldValue_v1,
) []FieldValue_v2 {
	fields := make([]FieldValue_v2, len(cfields))
	for i := range cfields {
		fields[i] = FieldValue_v2{
			Version:       C.dcgmFieldValue_version2,
			EntityGroupId: fieldEntityGroup,
			EntityID:      entityId,
			FieldID:       Short(cfields[i].fieldId),
			FieldType:     uint(cfields[i].fieldType),
			Status:        int(cfields[i].status),
			TS:            int64(cfields[i].ts),
			Value:         cfields[i].value,
			StringValue:   nil,
		}

		if uint(cfields[i].fieldType) == DCGM_FT_STRING {
			fields[i].StringValue = stringPtr((*C.char)(unsafe.Pointer(&cfields[i].value[0])))
		}
	}

	return fields
}

// Int64 returns the field value as an int64.
func (fv FieldValue_v2) Int64() int64 {
	return *(*int64)(unsafe.Pointer(&fv.Value[0]))
}

// Float64 returns the field value as a float64.
func (fv FieldValue_v2) Float64() float64 {
	return *(*float64)(unsafe.Pointer(&fv.Value[0]))
}

// String returns the field value as a string.
func (fv FieldValue_v2) String() string {
	return C.GoString((*C.char)(unsafe.Pointer(&fv.Value[0])))
}

// Blob returns the raw field value as a byte array.
func (fv FieldValue_v2) Blob() [4096]byte {
	return fv.Value
}

// FindFirstNonAsciiIndex returns the index of the first non-ASCII character in the byte array.
// Returns 4096 if no non-ASCII character is found.
func FindFirstNonAsciiIndex(value [4096]byte) int {
	for i := 0; i < 4096; i++ {
		if value[i] > unicode.MaxASCII || value[i] < 33 {
			return i
		}
	}

	return 4096
}

// Fv2_String returns the string value of a FieldValue_v2.
func Fv2_String(fv FieldValue_v2) string {
	if fv.FieldType == DCGM_FT_STRING {
		return *fv.StringValue
	} else {
		return string(fv.Value[:])
	}
}

// Fv2_Blob returns the raw field value of a FieldValue_v2 as a byte array.
func Fv2_Blob(fv FieldValue_v2) [4096]byte {
	return fv.Value
}

// ToFieldMeta converts a C DCGM field metadata structure to a Go FieldMeta struct.
// In case of an invalid fieldInfo pointer, it returns a zeroed FieldMeta.
func ToFieldMeta(fieldInfo C.dcgm_field_meta_p) FieldMeta {
	if fieldInfo == nil {
		return FieldMeta{}
	}
	return FieldMeta{
		FieldID:     Short(fieldInfo.fieldId),
		FieldType:   byte(fieldInfo.fieldType),
		Size:        byte(fieldInfo.size),
		Tag:         C.GoString((*C.char)(unsafe.Pointer(&fieldInfo.tag[0]))),
		Scope:       int(fieldInfo.scope),
		NvmlFieldID: int(fieldInfo.nvmlFieldId),
		EntityLevel: Field_Entity_Group(fieldInfo.entityLevel),
	}
}

// FieldGetByID retrieves field metadata for the specified field ID.
// Returns a zeroed FieldMeta if DCGM does not recognize the field ID.
func FieldGetByID(fieldId Short) (FieldMeta, error) {
	fieldInfo := C.DcgmFieldGetById(C.ushort(fieldId))
	if fieldInfo == nil {
		return FieldMeta{}, fmt.Errorf("field ID %d not recognized by DCGM", fieldId)
	}
	// If a fieldId is in the valid range 0..DCGM_FI_MAX_FIELDS but is not supported,
	// the field metadata will be zeroed instead of a null pointer returned.
	// The fieldType == 0 is an invalid type and an indication that the struct is zeroed.
	if fieldInfo.fieldType == 0 {
		return FieldMeta{}, fmt.Errorf("field ID %d is not supported by DCGM", fieldId)
	}
	return ToFieldMeta(fieldInfo), nil
}

// GetAllSupportedFieldsMetadata retrieves metadata for all supported DCGM fields.
// It returns a map of field IDs to their corresponding FieldMeta information.
// Unsupported fields are excluded from the resulting map.
func GetAllSupportedFieldsMetadata() map[Short]FieldMeta {
	fields := make(map[Short]FieldMeta)
	for _, id := range dcgmFields {
		if fieldInfo, err := FieldGetByID(id); err == nil {
			fields[id] = fieldInfo
		}
	}
	return fields
}

// FieldsInit initializes the DCGM fields module.
// Returns an integer status code.
func FieldsInit() int {
	return int(C.DcgmFieldsInit())
}

// FieldsTerm terminates the DCGM fields module.
// Returns an integer status code.
func FieldsTerm() int {
	return int(C.DcgmFieldsTerm())
}
