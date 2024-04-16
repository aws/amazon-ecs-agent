//go:build linux || darwin
// +build linux darwin

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fs

import (
	"golang.org/x/sys/unix"
)

// Info linux returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func Info(path string) (
	available int64, capacity int64, used int64,
	totalInodes int64, freeInodes int64, usedInodes int64,
	err error,
) {
	statfs := &unix.Statfs_t{}
	err = unix.Statfs(path, statfs)
	if err != nil {
		available = 0
		capacity = 0
		used = 0

		totalInodes = 0
		freeInodes = 0
		usedInodes = 0
		return
	}

	// Available is blocks available * fragment size
	available = int64(statfs.Bavail) * int64(statfs.Bsize)
	// Capacity is total block count * fragment size
	capacity = int64(statfs.Blocks) * int64(statfs.Bsize)
	// Used is block being used * fragment size (aka block size).
	used = (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	totalInodes = int64(statfs.Files)
	freeInodes = int64(statfs.Ffree)
	usedInodes = totalInodes - freeInodes
	return
}
