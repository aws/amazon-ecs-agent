// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package encoding

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/model"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"runtime/debug"
	"time"
)

var timeType reflect.Type = reflect.TypeOf(&time.Time{})

func encodeStruct(val reflect.Value) map[string]interface{} {
	t := val.Type()
	m := make(map[string]interface{}, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := field.Tag.Get("awsjson")
		if name == "" {
			continue
		}
		encodedVal := encode(val.Field(i))
		if encodedVal != nil {
			m[name] = encodedVal
		}
	}
	return m
}

func encodeTime(val reflect.Value) float64 {
	//Format: Seconds.Milliseconds
	timeVal, _ := val.Convert(timeType.Elem()).Interface().(time.Time)
	seconds := timeVal.Unix()
	ms := time.Duration(timeVal.Nanosecond()) * time.Nanosecond
	return float64(seconds) + ms.Seconds()
}

func encodeMap(val reflect.Value) interface{} {
	m := make(map[string]interface{}, 0)
	keys := val.MapKeys()
	if len(keys) == 0 {
		return nil
	}
	for _, key := range keys {
		value := val.MapIndex(key)
		// do Indirect so we can support map[*string]*string
		key = reflect.Indirect(key)
		value = reflect.Indirect(value)
		if str, ok := key.Interface().(string); ok {
			m[str] = encode(value)
		} else {
			panic(fmt.Errorf("Unexpected key type %T, want string", key.Interface()))
		}
	}
	return m
}

func encodeSlice(val reflect.Value) interface{} {
	if val.Len() == 0 {
		return nil
	}
	slice := make([]interface{}, val.Len())
	sliceIndex := 0
	for i := 0; i < len(slice); i++ {
		encodedValue := encode(val.Index(i))
		if encodedValue != nil {
			slice[sliceIndex] = encodedValue
			sliceIndex++
		}
	}
	return slice
}

func encode(val reflect.Value) interface{} {
	val = reflect.Indirect(val)
	if !val.IsValid() {
		return nil
	}
	t := val.Type()
	if t.ConvertibleTo(timeType.Elem()) {
		return encodeTime(val)
	}
	switch t.Kind() {
	case reflect.Ptr:
		return encode(reflect.Indirect(val))
	case reflect.Interface:
		return encode(val.Elem())
	case reflect.Struct:
		return encodeStruct(val)
	case reflect.Array, reflect.Slice:
		return encodeSlice(val)
	case reflect.Map:
		return encodeMap(val)
	}
	return val.Interface()
}

func decodeMap(in reflect.Value, t reflect.Type) reflect.Value {
	k := t.Kind()
	if k == reflect.Interface {
		return decodeMapIntoInterface(in, t)
	} else if k == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
		return decodeMapIntoStruct(in, t)
	} else if k == reflect.Map {
		return decodeMapIntoMap(in, t)
	}
	panic(fmt.Errorf("Unknown Kind: %s, type was %s", k, t))
}

func decodeMapIntoStruct(in reflect.Value, t reflect.Type) reflect.Value {
	var out reflect.Value
	var structT reflect.Type
	if t.Kind() == reflect.Ptr {
		out = reflect.New(t.Elem())
		structT = t.Elem()
	} else {
		out = reflect.New(t)
		structT = t
	}
	for i := 0; i < structT.NumField(); i++ {
		field := structT.Field(i)
		key := field.Tag.Get("awsjson")
		mapVal := in.MapIndex(reflect.ValueOf(key))
		if mapVal.IsValid() {
			//Key doesn't exist otherwise
			val := decode(mapVal, field.Type)
			outField := out.Elem().Field(i)
			outField.Set(val)
		}
	}
	return out
}

func decodeMapIntoInterface(in reflect.Value, t reflect.Type) reflect.Value {
	var shape model.Shape
	var err error
	if shape == nil {
		shape, err = model.GetShapeFromType(t)
		if err != nil {
			panic(err)
		}
	}
	out := reflect.ValueOf(shape.New()).Elem()
	return decodeMapIntoStruct(in, out.Type())
}

func decodeMapIntoMap(in reflect.Value, t reflect.Type) reflect.Value {
	out := reflect.MakeMap(t)
	outElemT := t.Elem()
	outKeyT := t.Key()
	for _, key := range in.MapKeys() {
		v := decode(in.MapIndex(key), outElemT)
		k := decode(key, outKeyT)
		out.SetMapIndex(k, v)
	}
	return out
}

func decodeSlice(in reflect.Value, t reflect.Type) reflect.Value {
	size := in.Len()
	out := reflect.MakeSlice(t, size, size)
	elemT := t.Elem()
	for i := 0; i < size; i++ {
		val := decode(in.Index(i), elemT)
		out.Index(i).Set(val)
	}
	return out
}

func decodeTime(in reflect.Value, t reflect.Type) reflect.Value {
	fractionalSeconds := in.Float()
	sec, ms := math.Modf(fractionalSeconds)
	nsec := ms * float64(time.Millisecond)
	timeVal := time.Unix(int64(sec), int64(nsec))
	return reflect.ValueOf(&timeVal).Convert(t)
}

func decode(in reflect.Value, t reflect.Type) reflect.Value {
	in = reflect.Indirect(in)
	if !in.IsValid() {
		return reflect.ValueOf(nil)
	}
	if in.Kind() == reflect.Interface {
		in = in.Elem()
	}
	if t.ConvertibleTo(timeType) {
		return decodeTime(in, t)
	}
	switch in.Kind() {
	case reflect.Map:
		return decodeMap(in, t)
	case reflect.Array, reflect.Slice:
		return decodeSlice(in, t)
	default:
		if t.Kind() == reflect.Ptr {
			return ptrTo(in.Convert(t.Elem()))
		}
		return in.Convert(t)
	}
}

func Marshal(obj interface{}) (b []byte, err error) {
	defer func() {
		if errObj := recover(); errObj != nil {
			err = fmt.Errorf("Error: %v.\nStack Trace: %s", errObj, string(debug.Stack()))
		}
	}()
	if obj == nil {
		return []byte(""), nil
	}
	val := reflect.Indirect(reflect.ValueOf(obj))
	if val.IsValid() && val.Type().Kind() != reflect.Struct {
		return nil, fmt.Errorf("Cannot marshal top-level obj that isn't a struct. Kind was %v", val.Type().Kind())
	}
	m := encode(val)
	return json.Marshal(m)
}

func Unmarshal(d []byte, obj interface{}) (err error) {
	defer func() {
		if errObj := recover(); errObj != nil {
			err = fmt.Errorf("Error: %v.\nStack Trace: %s", errObj, string(debug.Stack()))
		}
	}()
	var m map[string]interface{}
	err = json.Unmarshal(d, &m)
	if err != nil {
		return
	}
	targetType := reflect.TypeOf(obj).Elem()
	decoded := decode(reflect.ValueOf(m), targetType)
	var ok bool
	if err, ok = decoded.Interface().(error); ok && (!decoded.Type().AssignableTo(targetType)) {
		return
	} else {
		reflect.ValueOf(obj).Elem().Set(decoded)
		return
	}
}

func ptrTo(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v.Addr()
	} else {
		ptr := reflect.New(v.Type())
		reflect.Indirect(ptr).Set(v)
		return ptr
	}
}
