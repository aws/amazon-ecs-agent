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

package mobypkgwrapper

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// defaultSocketsPath is where Docker looks for plugin unix sockets.
const defaultSocketsPath = "/run/docker/plugins"

// Plugins discovers Docker plugins installed on the host.
type Plugins interface {
	Scan() ([]string, error)
}

type plugins struct {
}

// NewPlugins creates a new Plugins object
func NewPlugins() Plugins {
	return &plugins{}
}

// Scan returns the names of all plugins registered through the Docker plugin
// socket directory or the plugin spec directories. It mirrors the discovery
// performed by the Docker daemon's local plugin registry. The agent runs as
// root against the host daemon, so rootless-daemon spec paths are not
// considered.
func (*plugins) Scan() ([]string, error) {
	var names []string
	dirEntries, err := os.ReadDir(defaultSocketsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "error reading dir entries")
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
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

	for _, p := range specsPaths() {
		dirEntries, err = os.ReadDir(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(err, "error reading dir entries")
		}
		for _, entry := range dirEntries {
			if entry.IsDir() {
				infos, err := os.ReadDir(filepath.Join(p, entry.Name()))
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

			switch ext := filepath.Ext(entry.Name()); ext {
			case ".spec", ".json":
				names = append(names, strings.TrimSuffix(entry.Name(), ext))
			}
		}
	}
	return names, nil
}
