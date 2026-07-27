// The Scan() disk-discovery logic in this file is derived from the Docker
// (moby) Project's github.com/docker/docker/pkg/plugins package.
// The original code may be found at:
// https://github.com/moby/moby/blob/v25.0.6/pkg/plugins/discovery.go
//
// Copyright The Moby Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.
//
// Modifications are Copyright Amazon.com Inc. or its affiliates. All Rights
// Reserved.
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

// Package mobypkgwrapper provides a thin wrapper over the local Docker plugin
// discovery logic that the agent relies on. moby v29 no longer publishes
// github.com/docker/docker/pkg/plugins as a supported public module, so the
// single call the agent makes (LocalRegistry.Scan) is reimplemented here using
// only the standard library. See migration ticket 08 for the decision record.
package mobypkgwrapper

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// defaultSocketsPath is the directory in which locally-discoverable plugins
// register their unix sockets. The agent runs as root against the host daemon,
// so the rootless socket/spec paths that upstream moby also scans are
// intentionally omitted (see moby#43111).
const defaultSocketsPath = "/run/docker/plugins"

// Plugins wraps moby/pkg/plugins methods for testing
type Plugins interface {
	Scan() ([]string, error)
}

type plugins struct {
}

// NewPlugins creates a new Plugins object
func NewPlugins() Plugins {
	return &plugins{}
}

// Scan scans the local plugin socket and spec directories and returns the
// names of all plugins it discovers. It mirrors the non-rootless behaviour of
// the upstream moby LocalRegistry.Scan: a directory entry counts if it is a
// socket (or a <name>/<name>.sock) under the sockets path, or a .spec/.json
// file under a specs path.
func (*plugins) Scan() ([]string, error) {
	var names []string

	dirEntries, err := os.ReadDir(defaultSocketsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "error reading dir entries")
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
			// A plugin may register as <name>/<name>.sock rather than a
			// bare socket in the sockets directory.
			fi, err := os.Stat(filepath.Join(defaultSocketsPath, entry.Name(), entry.Name()+".sock"))
			if err != nil {
				continue
			}
			entry = fs.FileInfoToDirEntry(fi)
		}

		if entry.Type()&os.ModeSocket != 0 {
			names = append(names, strings.TrimSuffix(filepath.Base(entry.Name()), filepath.Ext(entry.Name())))
		}
	}

	for _, specsPath := range specsPaths() {
		dirEntries, err = os.ReadDir(specsPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(err, "error reading dir entries")
		}

		for _, entry := range dirEntries {
			if entry.IsDir() {
				infos, err := os.ReadDir(filepath.Join(specsPath, entry.Name()))
				if err != nil {
					continue
				}
				for _, info := range infos {
					if strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())) == entry.Name() {
						entry = info
						break
					}
				}
			}

			switch filepath.Ext(entry.Name()) {
			case ".spec", ".json":
				names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
			}
		}
	}

	return names, nil
}
