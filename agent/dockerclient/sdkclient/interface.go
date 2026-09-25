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

	"github.com/moby/moby/client"
)

// Client is an interface specifying the subset of
// github.com/moby/moby/client that the agent uses. It mirrors the moby v29
// client.Client method signatures exactly so that the concrete *client.Client
// satisfies it directly, with no adapter code (see migration ticket 09).
type Client interface {
	ClientVersion() string
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerInspect(ctx context.Context, container string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerTop(ctx context.Context, container string, options client.ContainerTopOptions) (client.ContainerTopResult, error)
	ContainerRemove(ctx context.Context, container string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerStart(ctx context.Context, container string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStats(ctx context.Context, container string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	ContainerStop(ctx context.Context, container string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ExecCreate(ctx context.Context, container string, options client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecStart(ctx context.Context, execID string, options client.ExecStartOptions) (client.ExecStartResult, error)
	ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error)
	DistributionInspect(ctx context.Context, image string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error)
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	ImageImport(ctx context.Context, source client.ImageImportSource, ref string, options client.ImageImportOptions) (client.ImageImportResult, error)
	ImageInspect(ctx context.Context, image string, inspectOpts ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImageLoad(ctx context.Context, input io.Reader, loadOpts ...client.ImageLoadOption) (client.ImageLoadResult, error)
	ImageList(ctx context.Context, options client.ImageListOptions) (client.ImageListResult, error)
	ImagePull(ctx context.Context, ref string, options client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageRemove(ctx context.Context, image string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error)
	ImageTag(ctx context.Context, options client.ImageTagOptions) (client.ImageTagResult, error)
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	PluginList(ctx context.Context, options client.PluginListOptions) (client.PluginListResult, error)
	VolumeCreate(ctx context.Context, options client.VolumeCreateOptions) (client.VolumeCreateResult, error)
	VolumeInspect(ctx context.Context, volumeID string, options client.VolumeInspectOptions) (client.VolumeInspectResult, error)
	VolumeRemove(ctx context.Context, volumeID string, options client.VolumeRemoveOptions) (client.VolumeRemoveResult, error)
	ServerVersion(ctx context.Context, options client.ServerVersionOptions) (client.ServerVersionResult, error)
	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
}
