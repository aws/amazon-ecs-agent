package apparmor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/aaparser"
	aaprofile "github.com/docker/docker/profiles/apparmor"
)

const (
	ECSDefaultProfileName = "ecs-default"
	appArmorProfileDir    = "/etc/apparmor.d"
)

const ecsDefaultProfile = `
#include <tunables/global>

profile ecs-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  network inet, # Allow IPv4 traffic
  network inet6, # Allow IPv6 traffic

  capability net_admin, # Allow network configuration
  capability sys_admin, # Allow ECS Agent to invoke the setns system call
  capability dac_override, # Allow ECS Agent to file read, write, and execute permission
 
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

var (
	isProfileLoaded = aaprofile.IsLoaded
	loadPath        = aaparser.LoadProfile
	createFile      = os.Create
)

// LoadDefaultProfile ensures the default profile to be loaded with the given name.
// Returns nil error if the profile is already loaded.
func LoadDefaultProfile(profileName string) error {
	yes, err := isProfileLoaded(profileName)
	if yes {
		return nil
	}
	if err != nil {
		return err
	}

	f, err := createFile(filepath.Join(appArmorProfileDir, profileName))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(ecsDefaultProfile)
	if err != nil {
		return err
	}
	path := f.Name()

	if err := loadPath(path); err != nil {
		return fmt.Errorf("error loading apparmor profile %s: %w", path, err)
	}
	return nil
}
