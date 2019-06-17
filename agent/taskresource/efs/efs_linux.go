// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package efs

import (
	"errors"
	"runtime"
	"time"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const (
	// TODO: break this out into a better options stream (IE, we will need overrides when specified in the service)
	defaultOptsForEFS = "rsize=1048576,wsize=1048576,timeo=10,hard,retrans=2,noresvport,vers=4"
	defaultSource     = ":/"
	fsTypeNFS         = "nfs"
	mountFlags        = 0
	unmountFlags      = 0
	mountTimeout      = time.Minute
)

var (
	errNoIPAddress    = errors.New("no IP Address set for nfs mount")
	errMountTimeout   = errors.New("mount operation timed out")
	errUnmountTimeout = errors.New("unmount operation timed out")
)

var (
	mountSyscall        = unix.Mount
	unmountSyscall      = unix.Unmount
	setNamespaceSyscall = netns.Set
	getNamespaceHelper  = netns.GetFromPath
)

// NFSMount uses the mount syscall to create a local nfs mount. It can optionally take a network namespace, which forces
// the mount to use the networking configuration applied to the other namespace.
type NFSMount struct {
	IPAddress       string
	TargetDirectory string
	SourceDirectory string
	NamespacePath   string
}

// Mount creates the nfs mount, placing it at TargetDirectory on the host
func (nm *NFSMount) Mount() error {
	if nm.IPAddress == "" {
		return errNoIPAddress
	}

	timeout := time.NewTimer(mountTimeout)
	defer timeout.Stop()

	mountEvent := make(chan error)
	go func() {
		mountEvent <- nm.doMount()
	}()

	select {
	case err := <-mountEvent:
		return err
	case <-timeout.C:
		return errMountTimeout
	}
}

// doMount handles the core mount logic. The main Mount() method spawns this
// in a goroutine
func (nm *NFSMount) doMount() error {
	if nm.NamespacePath != "" {
		runtime.LockOSThread()
		if err := nm.setNameSpace(); err != nil {
			return err
		}
	}

	opts := defaultOptsForEFS + ",addr=" + nm.IPAddress

	// NFS expects the source to appear like ${IP}:/${SourceDirectory}
	source := nm.IPAddress + defaultSource
	if nm.SourceDirectory != "" {
		source = source + nm.SourceDirectory
	}

	return mountSyscall(source, nm.TargetDirectory, fsTypeNFS, mountFlags, opts)
}

// Unmount removes the nfs mount from the host.
func (nm *NFSMount) Unmount() error {
	timeout := time.NewTimer(mountTimeout)
	defer timeout.Stop()

	mountEvent := make(chan error)
	go func() {
		mountEvent <- unmountSyscall(nm.TargetDirectory, unmountFlags)
	}()

	select {
	case err := <-mountEvent:
		return err
	case <-timeout.C:
		return errUnmountTimeout
	}
}

func (nm *NFSMount) setNameSpace() error {
	handle, err := getNamespaceHelper(nm.NamespacePath)
	if err != nil {
		return err
	}

	return setNamespaceSyscall(handle)
}
