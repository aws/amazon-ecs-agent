// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package apparmor

import (
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/ecs-init/config"
	"github.com/docker/docker/pkg/aaparser"
	aaprofile "github.com/docker/docker/profiles/apparmor"
)

const (
	ECSAgentDefaultProfileName = config.ECSAgentAppArmorDefaultProfileName
	appArmorProfileDir         = "/etc/apparmor.d"
)

type profile struct {
	IncludeTunablesGlobal   bool
	IncludeAbstractionsBase bool
}

const ecsAgentDefaultProfile = `
{{if .IncludeTunablesGlobal}}
#include <tunables/global>
{{else}}
@{PROC}=/proc/
{{end}}

profile ecs-agent-default flags=(attach_disconnected,mediate_deleted) {
  {{if .IncludeAbstractionsBase}}
  #include <abstractions/base>
  {{end}}

  network inet,
  network inet6,
  network netlink,
  network unix,
  capability,
  file,
  umount,
  # Host (privileged) processes may send signals to container processes.
  signal (receive) peer=unconfined,
  # Container processes may send signals amongst themselves.
  signal (send,receive) peer=ecs-agent-default,

  # ECS agent requires DBUS send
  dbus (send,receive) bus=system,

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
  ptrace (trace,read,tracedby,readby) peer=ecs-agent-default,
  ptrace (trace,read,tracedby,readby) peer=docker-default,
}
`

var (
	isProfileLoaded = aaprofile.IsLoaded
	loadPath        = aaparser.LoadProfile
	createFile      = os.Create
	statFile        = os.Stat
)

// LoadDefaultProfile ensures the default profile to be loaded with the given name.
// Returns nil error if the profile is already loaded.
func LoadDefaultProfile(profileName string) error {
	_, err := isProfileLoaded(profileName)
	if err != nil {
		return err
	}

	includeTunablesGlobal, err := fileExists(filepath.Join(appArmorProfileDir, "tunables/global"))
	if err != nil {
		return err
	}

	includeAbstractionsBase, err := fileExists(filepath.Join(appArmorProfileDir, "abstractions/base"))
	if err != nil {
		return err
	}

	f, err := createFile(filepath.Join(appArmorProfileDir, profileName))
	if err != nil {
		return err
	}
	defer f.Close()

	t := template.Must(template.New("profile").Parse(ecsAgentDefaultProfile))
	err = t.Execute(f, profile{
		IncludeTunablesGlobal:   includeTunablesGlobal,
		IncludeAbstractionsBase: includeAbstractionsBase,
	})
	if err != nil {
		return err
	}

	path := f.Name()
	if err := loadPath(path); err != nil {
		return fmt.Errorf("error loading apparmor profile %s: %w", path, err)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := statFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
