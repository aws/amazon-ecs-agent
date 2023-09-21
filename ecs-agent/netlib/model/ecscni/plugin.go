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

package ecscni

import (
	"context"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	NETNS_PATH_DEFAULT = "/var/run/netns"
	NETNS_PROC_FORMAT  = "/proc/%d/task/%d/ns/net"

	NsFileMode = 0444
)

// CNI defines the plugin invocation interface
type CNI interface {
	// Add calls the plugin add command with given configuration
	Add(context.Context, PluginConfig) (types.Result, error)
	// Del calls the plugin del command with given configuration
	Del(context.Context, PluginConfig) error
	// Version calls the version command of plugin
	Version(string) (string, error)
}

// Config is a general interface represents all kinds of plugin configs
type Config interface {
	String() string
}

// CNIPluginVersion is used to convert the JSON output of the
// '--version' command into a string
type CNIPluginVersion struct {
	Version string `json:"version"`
	Dirty   bool   `json:"dirty"`
	Hash    string `json:"gitShortHash"`
}

// String returns the version information as formatted string
func (v *CNIPluginVersion) String() string {
	ver := ""
	if v.Dirty {
		ver = "@"
	}

	return ver + v.Hash + "-" + v.Version
}
