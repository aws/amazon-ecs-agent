package model

import (
	"reflect"
	"testing"
)

type testShape struct {
}

func TestRegisterShape(t *testing.T) {
	var i testShape
	typeOf := reflect.TypeOf(&i)
	RegisterShape("testShape", typeOf, func() interface{} {
		return testShape{}
	})
	shape, err := GetShapeFromType(reflect.Indirect(reflect.ValueOf(testShape{})).Type())
	if err != nil {
		t.Errorf("%s", err)
	}
	if _, ok := shape.New().(testShape); !ok {
		t.Errorf(`New("bar") = %t, expected %t`, shape, testShape{})
	}
}
