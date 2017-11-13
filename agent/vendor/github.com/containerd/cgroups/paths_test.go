package cgroups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticPath(t *testing.T) {
	path := StaticPath("test")
	p, err := path("")
	if err != nil {
		t.Fatal(err)
	}
	if p != "test" {
		t.Fatalf("expected static path of \"test\" but received %q", p)
	}
}

func TestSelfPath(t *testing.T) {
	paths, err := parseCgroupFile("/proc/self/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	dp := strings.TrimPrefix(paths["devices"], "/")
	path := NestedPath("test")
	p, err := path("devices")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join("/", dp, "test") {
		t.Fatalf("expected self path of %q but received %q", filepath.Join("/", dp, "test"), p)
	}
}

func TestPidPath(t *testing.T) {
	paths, err := parseCgroupFile("/proc/self/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	dp := strings.TrimPrefix(paths["devices"], "/")
	path := PidPath(os.Getpid())
	p, err := path("devices")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join("/", dp) {
		t.Fatalf("expected self path of %q but received %q", filepath.Join("/", dp), p)
	}
}

func TestRootPath(t *testing.T) {
	p, err := RootPath(Cpu)
	if err != nil {
		t.Error(err)
		return
	}
	if p != "/" {
		t.Errorf("expected / but received %q", p)
	}
}
