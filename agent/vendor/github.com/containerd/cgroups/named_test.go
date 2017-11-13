package cgroups

import "testing"

func TestNamedNameValue(t *testing.T) {
	n := NewNamed("/sys/fs/cgroup", "systemd")
	if n.name != "systemd" {
		t.Fatalf("expected name %q to be systemd", n.name)
	}
}

func TestNamedPath(t *testing.T) {
	n := NewNamed("/sys/fs/cgroup", "systemd")
	path := n.Path("/test")
	if expected := "/sys/fs/cgroup/systemd/test"; path != expected {
		t.Fatalf("expected %q but received %q from named cgroup", expected, path)
	}
}
