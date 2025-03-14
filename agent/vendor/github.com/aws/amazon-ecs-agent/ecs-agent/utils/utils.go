// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.
package utils

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"golang.org/x/exp/constraints"
)

func ZeroOrNil(obj interface{}) bool {
	value := reflect.ValueOf(obj)
	if !value.IsValid() {
		return true
	}
	if obj == nil {
		return true
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return value.Len() == 0
	}
	zero := reflect.Zero(reflect.TypeOf(obj))
	if !value.Type().Comparable() {
		return false
	}
	if obj == zero.Interface() {
		return true
	}
	return false
}

// Uint16SliceToStringSlice converts a slice of type uint16 to a slice of type
// string. It uses strconv.Itoa on each element
func Uint16SliceToStringSlice(slice []uint16) []string {
	stringSlice := make([]string, len(slice))
	for i, el := range slice {
		str := strconv.Itoa(int(el))
		stringSlice[i] = str
	}
	return stringSlice
}

// Int32PtrToIntPtr converts a *int32 to *int.
func Int32PtrToIntPtr(int32ptr *int32) *int {
	if int32ptr == nil {
		return nil
	}
	return aws.Int(int(aws.ToInt32(int32ptr)))
}

// Int64PtrToInt32Ptr converts a *int64 to *int32.
func Int64PtrToInt32Ptr(int64ptr *int64) *int32 {
	if int64ptr == nil {
		return nil
	}
	return aws.Int32(int32(aws.ToInt64(int64ptr)))
}

// MaxNum returns the maximum value between two numbers.
func MaxNum[T constraints.Integer | constraints.Float](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Adapted from
// - https://github.com/aws/aws-sdk-go/blob/main/aws/awsutil/prettify_test.go
// - https://github.com/aws/aws-sdk-go-v2/blob/main/internal/awsutil/prettify.go
// "sensitive" tags aren't known at runtime in sdk v2; need to manually redact sensitive fields
// Prettify returns the string representation of a value.
func Prettify(i interface{}, sensitiveFields ...string) string {
	var buf bytes.Buffer
	sensitiveFieldsMap := make(map[string]bool)
	if len(sensitiveFields) > 0 {
		for _, field := range sensitiveFields {
			sensitiveFieldsMap[field] = true
		}
	}
	prettify(reflect.ValueOf(i), 0, &buf, sensitiveFieldsMap)
	return buf.String()
}

// prettify will recursively walk value v to build a textual
// representation of the value.
func prettify(v reflect.Value, indent int, buf *bytes.Buffer, sensitiveFields map[string]bool) {
	isPtr := false
	for v.Kind() == reflect.Ptr {
		isPtr = true
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		strtype := v.Type().String()
		if strtype == "time.Time" {
			fmt.Fprintf(buf, "%s", v.Interface())
			break
		} else if strings.HasPrefix(strtype, "io.") {
			buf.WriteString("<buffer>")
			break
		}

		if isPtr {
			buf.WriteRune('&')
		}
		buf.WriteString("{\n")

		names := []string{}
		for i := 0; i < v.Type().NumField(); i++ {
			name := v.Type().Field(i).Name
			f := v.Field(i)
			if name[0:1] == strings.ToLower(name[0:1]) {
				continue // ignore unexported fields
			}
			if (f.Kind() == reflect.Ptr || f.Kind() == reflect.Slice || f.Kind() == reflect.Map) && f.IsNil() {
				continue // ignore unset fields
			}
			names = append(names, name)
		}

		for i, n := range names {
			val := v.FieldByName(n)

			buf.WriteString(strings.Repeat(" ", indent+2))
			buf.WriteString(n + ": ")

			if _, ok := sensitiveFields[n]; ok {
				buf.WriteString("<sensitive>")
			} else {
				prettify(val, indent+2, buf, sensitiveFields)
			}

			if i < len(names)-1 {
				buf.WriteString(",\n")
			}
		}

		buf.WriteString("\n" + strings.Repeat(" ", indent) + "}")
	case reflect.Slice:
		strtype := v.Type().String()
		if strtype == "[]uint8" {
			fmt.Fprintf(buf, "<binary> len %d", v.Len())
			break
		}

		nl, id, id2 := "\n", strings.Repeat(" ", indent), strings.Repeat(" ", indent+2)
		if isPtr {
			buf.WriteRune('&')
		}
		buf.WriteString("[" + nl)
		for i := 0; i < v.Len(); i++ {
			buf.WriteString(id2)
			prettify(v.Index(i), indent+2, buf, sensitiveFields)

			if i < v.Len()-1 {
				buf.WriteString("," + nl)
			}
		}

		buf.WriteString(nl + id + "]")
	case reflect.Map:
		if isPtr {
			buf.WriteRune('&')
		}
		buf.WriteString("{\n")

		for i, k := range v.MapKeys() {
			buf.WriteString(strings.Repeat(" ", indent+2))
			buf.WriteString(k.String() + ": ")
			prettify(v.MapIndex(k), indent+2, buf, sensitiveFields)

			if i < v.Len()-1 {
				buf.WriteString(",\n")
			}
		}

		buf.WriteString("\n" + strings.Repeat(" ", indent) + "}")
	default:
		if !v.IsValid() {
			fmt.Fprint(buf, "<invalid value>")
			return
		}

		for v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
		}

		if v.Kind() == reflect.Ptr || v.Kind() == reflect.Struct || v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
			prettify(v, indent, buf, sensitiveFields)
			return
		}

		format := "%v"
		switch v.Interface().(type) {
		case string:
			format = "%q"
		case io.ReadSeeker, io.Reader:
			format = "buffer(%p)"
		}
		fmt.Fprintf(buf, format, v.Interface())
	}
}
