package dcgm

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

	// fieldValuesSliceSize is the number of fields in the DCGM.
	// See: https://docs.nvidia.com/datacenter/dcgm/latest/dcgm-api/dcgm-api-field-ids.html
	fieldValuesSliceSize = 175
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
	return
}

// FieldGroupDestroy destroys a previously created field group.
// Returns an error if the group cannot be destroyed.
func FieldGroupDestroy(fieldsGroup FieldHandle) (err error) {
	result := C.dcgmFieldGroupDestroy(handle.handle, fieldsGroup.handle)
	if err = errorString(result); err != nil {
		err = fmt.Errorf("error destroying DCGM fields group: %s", err)
	}

	return
}

// WatchFields starts monitoring the specified fields for a GPU.
// gpuId is the ID of the GPU to monitor.
// fieldsGroup is the handle of the field group to watch.
// groupName is a name for the watch group.
// Returns a group handle and any error encountered.
func WatchFields(gpuID uint, fieldsGroup FieldHandle, groupName string) (groupId GroupHandle, err error) {
	group, err := CreateGroup(groupName)
	if err != nil {
		return
	}

	err = AddToGroup(group, gpuID)
	if err != nil {
		return
	}

	result := C.dcgmWatchFields(handle.handle, group.handle, fieldsGroup.handle, C.longlong(defaultUpdateFreq),
		C.double(defaultMaxKeepAge), C.int(defaultMaxKeepSamples))
	if err = errorString(result); err != nil {
		return groupId, fmt.Errorf("error watching fields: %s", err)
	}

	_ = UpdateAllFields()
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
		if slice, ok := v.([]T); ok && cap(slice) >= size {
			return slice[:size]
		}
	}
	return make([]T, size)
}

func releaseSlice[T any](pool *sync.Pool, slice []T) {
	pool.Put(&slice)
}

func acquireFieldValueSlice(size int) []C.dcgmFieldValue_v1 {
	return acquireSlice[C.dcgmFieldValue_v1](&fieldValuePool, size)
}

func releaseFieldValueSlice(slice []C.dcgmFieldValue_v1) {
	releaseSlice(&fieldValuePool, slice)
}

func acquireFieldValueV2Slice(size int) []C.dcgmFieldValue_v2 {
	return acquireSlice[C.dcgmFieldValue_v2](&fieldValueV2Pool, size)
}

func releaseFieldValueV2Slice(slice []C.dcgmFieldValue_v2) {
	releaseSlice(&fieldValueV2Pool, slice)
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
	slice := []byte{uint8(parentType), uint8(index), uint8(parentId), 0}
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
func ToFieldMeta(fieldInfo C.dcgm_field_meta_p) FieldMeta {
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
func FieldGetByID(fieldId Short) FieldMeta {
	return ToFieldMeta(C.DcgmFieldGetById(C.ushort(fieldId)))
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
