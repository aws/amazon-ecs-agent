package api

import (
	"reflect"
	"testing"
)

func TestOverridden(t *testing.T) {
	container := &Container{
		Name:          "name",
		Image:         "image",
		Command:       []string{"foo", "bar"},
		Cpu:           1,
		Memory:        1,
		Links:         []string{},
		Ports:         []PortBinding{PortBinding{10, 10, ""}},
		Overrides:     ContainerOverrides{},
		DesiredStatus: ContainerRunning,
		AppliedStatus: ContainerRunning,
		KnownStatus:   ContainerRunning,
	}

	overridden := container.Overridden()
	// No overrides, should be identity
	if !reflect.DeepEqual(container, overridden) {
		t.Error("Were not equal")
	}
	if container == overridden {
		t.Error("Were pointer equal")
	}
	overridden.Name = "mutated"

	if container.Name != "name" {
		t.Error("Should make a copy")
	}
}
