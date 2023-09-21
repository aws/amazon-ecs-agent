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
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

const (
	versionCommand = "--version"
)

// CNIClient is the client to invoke the plugin
type cniClient struct {
	pluginPath []string
	cni        libcni.CNI
}

// NewCNIClient creates a new CNIClient
func NewCNIClient(paths []string) CNI {
	return &cniClient{
		pluginPath: paths,
		cni:        libcni.NewCNIConfig(paths, nil),
	}
}

// Add invokes the plugin with add command
func (c *cniClient) Add(ctx context.Context, config PluginConfig) (types.Result, error) {
	rt := BuildRuntimeConfig(config)
	net, err := BuildNetworkConfig(config)
	if err != nil {
		return nil, err
	}

	return c.cni.AddNetwork(ctx, net, rt)
}

// Del invokes the vpc-branch-eni plugin with del command
func (c *cniClient) Del(ctx context.Context, config PluginConfig) error {
	rt := BuildRuntimeConfig(config)
	net, err := BuildNetworkConfig(config)
	if err != nil {
		return err
	}

	return c.cni.DelNetwork(ctx, net, rt)
}

func (c *cniClient) Version(plugin string) (string, error) {
	pathsFromEnv := strings.Split(os.Getenv("PATH"), ":")
	file, err := invoke.FindInPath(plugin, append(c.pluginPath, pathsFromEnv...))
	if err != nil {
		return "", errors.Wrapf(err, "unable to find plugin: %s", plugin)
	}

	cmd := exec.Command(file, versionCommand)
	versionInfo, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "unable to get version info for plugin: %s", plugin)
	}

	version := &CNIPluginVersion{}
	err = json.Unmarshal(versionInfo, version)
	if err != nil {
		return "", errors.Wrapf(err, "unable to unmarshal version info for plugin: %s", plugin)
	}

	return version.String(), nil
}
