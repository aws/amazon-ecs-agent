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
	"reflect"
	"testing"
	"time"
)

type encodeTest struct {
	Foo *string     `awsjson:"foo"`
	Bar *string     `awsjson:"bar"`
	Baz *encodeTest `awsjson:"baz"`
}

func TestEncode(t *testing.T) {
	amazon, ec2, container, service := "Amazon", "EC2", "Container", "Service"
	val := reflect.ValueOf(&encodeTest{&amazon, &ec2, &encodeTest{&container, &service, nil}})
	m, ok := encode(val).(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid type received from encode, want map[string]interface{}, got %T", encode(val))
	}
	if m["foo"] != amazon {
		t.Errorf(`m["foo"] = "%v", want "%s"`, m["foo"], amazon)
	}
	if m["bar"] != ec2 {
		t.Errorf(`m["bar"] = %v", want "%s"`, m["bar"], ec2)
	}
	if baz, ok := m["baz"].(map[string]interface{}); ok {
		if baz["foo"] != container {
			t.Errorf(`baz["foo"] = "%v", want "%s"`, baz["foo"], container)
		}
		if baz["bar"] != service {
			t.Errorf(`baz["bar"] = %v, want "%s"`, baz["baz"], service)
		}
		if baz["baz"] != nil {
			t.Errorf(`baz["baz"] = %v, want nil`, baz["bar"])
		}
	} else {
		t.Errorf(`m["baz"] = %t, want map[string]interface{}`, m["bar"])
	}
}

type timeTest *time.Time

func TestTime(t *testing.T) {
	origTime := time.Now()
	var test timeTest = timeTest(&origTime)
	val := reflect.ValueOf(test)
	if f, ok := encode(val).(float64); !ok {
		t.Fatalf("Invalid Type received from encode, want float64, got %T", encode(val))
	} else if int64(f) != origTime.Unix() {
		t.Fatalf("Wrong time received from encode, got %v, want %v", int64(f), origTime.Unix())
	}
	var timeTestVal timeTest = new(time.Time)
	timeTestType := reflect.TypeOf(timeTestVal)
	newVal := decode(reflect.ValueOf(encode(val)), timeTestType).Interface()
	if timeVal, ok := newVal.(timeTest); !ok {
		t.Fatalf("Invalid Type received from decode, want timeTest, got %T", newVal)
	} else if (*time.Time)(timeVal).Unix() != origTime.Unix() {
		t.Fatalf("Wrong time received from decode, got %v, want %v", time.Time(*timeVal).Unix(), origTime.Unix())
	}

}
