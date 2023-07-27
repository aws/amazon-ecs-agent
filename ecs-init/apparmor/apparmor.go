package apparmor

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	ECSDefaultProfileName = "ecs-default"
	appArmorProfileDir    = "/etc/apparmor.d"
)

const ecsDefaultProfile = `
#include <tunables/global>

profile ecs-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  network,
  capability,
  file,
  umount,
  # Host (privileged) processes may send signals to container processes.
  signal (receive) peer=unconfined,
  # Container processes may send signals amongst themselves.
  signal (send,receive) peer=ecs-default,

  # ECS agent requires DBUS send
  dbus (send) bus=system,

  deny @{PROC}/* w,   # deny write for all files directly in /proc (not in a subdir)
  # deny write to files not in /proc/<number>/** or /proc/sys/**
  deny @{PROC}/{[^1-9],[^1-9][^0-9],[^1-9s][^0-9y][^0-9s],[^1-9][^0-9][^0-9][^0-9/]*}/** w,
  deny @{PROC}/sys/[^k]** w,  # deny /proc/sys except /proc/sys/k* (effectively /proc/sys/kernel)
  deny @{PROC}/sys/kernel/{?,??,[^s][^h][^m]**} w,  # deny everything except shm* in /proc/sys/kernel/
  deny @{PROC}/sysrq-trigger rwklx,
  deny @{PROC}/kcore rwklx,

  deny mount,

  deny /sys/[^f]*/** wklx,
  deny /sys/f[^s]*/** wklx,
  deny /sys/fs/[^c]*/** wklx,
  deny /sys/fs/c[^g]*/** wklx,
  deny /sys/fs/cg[^r]*/** wklx,
  deny /sys/firmware/** rwklx,
  deny /sys/kernel/security/** rwklx,

  # suppress ptrace denials when using 'docker ps' or using 'ps' inside a container
  ptrace (trace,read,tracedby,readby) peer=ecs-default,
}
`

// LoadDefaultProfile ensures the default profile to be loaded with the given name.
// Returns nil error if the profile is already loaded.
func LoadDefaultProfile(profileName string) error {
	yes, err := isLoaded(profileName)
	if err != nil {
		return err
	}
	if yes {
		return nil
	}

	f, err := os.Create(filepath.Join(appArmorProfileDir, profileName))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(ecsDefaultProfile)
	if err != nil {
		return err
	}
	path := f.Name()

	if err := load(path); err != nil {
		return fmt.Errorf("load apparmor profile %s: %w", path, err)
	}
	return nil
}
