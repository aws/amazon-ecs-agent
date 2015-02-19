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

package model

import (
	"errors"
	"fmt"
	"reflect"
)

type Shape interface {
	Name() string
	Type() reflect.Type
	New() interface{}
}

var shapes map[reflect.Type]Shape = map[reflect.Type]Shape{}

type shape struct {
	name        string             //The name of this shape
	t           reflect.Type       //The Type this shape represents
	constructor func() interface{} //A function that returns the default value for the shape
}

func GetShapeFromType(t reflect.Type) (Shape, error) {
	s, ok := shapes[t]
	if ok {
		return s, nil
	}
	return nil, errors.New("No Shape Registered for Type " + t.String())
}

func RegisterShape(name string, interfaceType reflect.Type, constructor func() interface{}) error {
	_, ok := shapes[interfaceType]
	if !ok {
		shape := &shape{
			name:        name,
			t:           interfaceType,
			constructor: constructor,
		}
		shapes[interfaceType.Elem()] = shape
		implType := reflect.Indirect(reflect.ValueOf(constructor())).Type()
		shapes[implType] = shape
		return nil
	}
	return fmt.Errorf("Shape already registered with the name %s", name)
}

func (s *shape) Name() string {
	return s.name
}

func (s *shape) Type() reflect.Type {
	return s.t
}

func (s *shape) New() interface{} {
	return s.constructor()
}

var strType reflect.Type = reflect.TypeOf("")

func ErrorMessage(e interface{}) string {
	val := reflect.ValueOf(e)
	name := val.Type().String()
	//Try to call a "Message() string" function
	method := val.MethodByName("Message")
	if method.IsValid() {
		output := method.Call([]reflect.Value{})

		message := output[0]
		if message.Type().ConvertibleTo(strType) {
			messageStr, _ := message.Convert(strType).Interface().(string)
			v := fmt.Sprintf("%s: %s", name, messageStr)
			return v
		}
	}
	return name
}
