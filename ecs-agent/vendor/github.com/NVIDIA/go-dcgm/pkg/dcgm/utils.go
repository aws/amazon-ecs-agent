package dcgm

/*
#include <stdlib.h>
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"math"
	"unsafe"
)

const (
	dcgmInt32Blank = 0x7ffffff0         // 2147483632
	dcgmInt64Blank = 0x7ffffffffffffff0 // 9223372036854775792
)

func uintPtr(c C.uint) *uint {
	i := uint(c)
	return &i
}

func uint64Ptr(c C.longlong) *uint64 {
	i := uint64(c)
	return &i
}

func int64Ptr(c C.longlong) *int64 {
	i := int64(c)
	return &i
}

func toInt64(c C.longlong) int64 {
	i := int64(c)
	return i
}

func dblToFloat(val C.double) *float64 {
	i := float64(val)
	return &i
}

func stringPtr(c *C.char) *string {
	s := C.GoString(c)
	return &s
}

// Error represents an error returned by the DCGM library
type Error struct {
	msg  string         // description of error
	Code C.dcgmReturn_t // dcgmReturn_t value of error
}

func (e *Error) Error() string { return e.msg }

func errorString(result C.dcgmReturn_t) error {
	if result == C.DCGM_ST_OK {
		return nil
	}
	err := C.GoString(C.errorString(result))
	return fmt.Errorf("%v", err)
}

func freeCString(cStr *C.char) {
	C.free(unsafe.Pointer(cStr))
}

// IsInt32Blank checks if an integer value represents DCGM's "blank" or sentinel value (0x7ffffff0).
// These values indicate that no valid data is available for the field.
func IsInt32Blank(value int) bool {
	return value >= dcgmInt32Blank
}

// IsInt64Blank checks if an integer value represents DCGM's "blank" or sentinel value (0x7ffffffffffffff0).
// These values indicate that no valid data is available for the field.
func IsInt64Blank(value int64) bool {
	return value >= dcgmInt64Blank
}

func makeVersion1(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 1<<24)
	return version
}

func makeVersion2(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 2<<24)
	return version
}

func makeVersion3(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 3<<24)
	return version
}

func makeVersion4(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 4<<24)
	return version
}

func makeVersion5(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 5<<24)
	return version
}

func makeVersion12(struct_type uintptr) C.uint {
	version := C.uint(struct_type | 12<<24)
	return version
}

func roundFloat(f *float64) *float64 {
	var val float64
	if f != nil {
		val = math.Round(*f)
	}
	return &val
}
