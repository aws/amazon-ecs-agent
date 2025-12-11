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

// Package sdkclient contains an interface for moby matching the
// subset used by the agent
package sdkclient

import (
	"context"
	"io"

	"github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Client is an interface specifying the subset of
// github.com/docker/docker/client that the agent uses.
type Client interface {
	ClientVersion() string
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig,
		networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerTop(ctx context.Context, containerID string, arguments []string) (container.TopResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerExecCreate(ctx context.Context, container string, config container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
	DistributionInspect(ctx context.Context, imageRef, encodedRegistryAuth string) (registry.DistributionInspect, error)
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	ImageImport(ctx context.Context, source image.ImportSource, ref string,
		options image.ImportOptions) (io.ReadCloser, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (image.InspectResponse, []byte, error)
	ImageLoad(ctx context.Context, input io.Reader,  _ ...client.ImageLoadOption) (image.LoadResponse, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse,
		error)
	ImageTag(ctx context.Context, source, target string) error
	Ping(ctx context.Context) (types.Ping, error)
	PluginList(ctx context.Context, filter filters.Args) (types.PluginsListResponse, error)
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	ServerVersion(ctx context.Context) (types.Version, error)
	Info(ctx context.Context) (system.Info, error)
}
