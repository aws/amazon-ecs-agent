package api

import (
	"reflect"
	"testing"
)

func taskN(n int) *Task {
	return &Task{
		Arn: string(n),
	}
}

func TestRemoveFromTaskArray(t *testing.T) {
	arr := []*Task{taskN(1), taskN(2), taskN(3)}

	arr2 := RemoveFromTaskArray(arr, -1)
	if !reflect.DeepEqual(arr, arr2) {
		t.Error("Index -1 out of bounds altered arr")
	}

	arr2 = RemoveFromTaskArray(arr, 3)
	if !reflect.DeepEqual(arr, arr2) {
		t.Error("Index 3 out of bounds altered arr")
	}

	arr = RemoveFromTaskArray(arr, 2)
	if !reflect.DeepEqual(arr, []*Task{taskN(1), taskN(2)}) {
		t.Error("Last element not removed")
	}
	arr = RemoveFromTaskArray(arr, 0)
	if !reflect.DeepEqual(arr, []*Task{taskN(2)}) {
		t.Error("First element not removed")
	}
	arr = RemoveFromTaskArray(arr, 0)
	if !reflect.DeepEqual(arr, []*Task{}) {
		t.Error("First element not removed")
	}
	arr = RemoveFromTaskArray(arr, 0)
	if !reflect.DeepEqual(arr, []*Task{}) {
		t.Error("Removing from empty arr changed it")
	}
}
