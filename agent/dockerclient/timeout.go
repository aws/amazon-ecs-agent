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

package dockerclient

import "time"

// Timelimits for docker operations enforced above docker
const (
	// ListImagesTimeout is the timeout for the ListImages API
	ListImagesTimeout = 10 * time.Minute
	// LoadImageTimeout is the timeout for the LoadImage API. It's set
	// to much lower value than pullImageTimeout as it involves loading
	// image from either a file or STDIN calls involved.
	// This value is set based on benchmarking the time of loading the pause container image,
	// since it's currently only used to load that image. If this is to be used to load any
	// other image in the future, additional benchmarking will be required.
	// Per benchmark, 2 min is roughly 4x of the worst case (29s) across 400 image loads on
	// al1/al2/al2gpu/al2arm with smallest instance type available (t2.nano/a1.medium) and
	// burst balance = 0.
	LoadImageTimeout = 2 * time.Minute
	// RemoveImageTimeout is the timeout for the RemoveImage API.
	RemoveImageTimeout = 3 * time.Minute
	// ListContainersTimeout is the timeout for the ListContainers API.
	ListContainersTimeout = 10 * time.Minute
	// InspectContainerTimeout is the timeout for the InspectContainer API.
	InspectContainerTimeout = 30 * time.Second
	// TopContainerTimeout is the timeout for the TopContainer API.
	TopContainerTimeout = 30 * time.Second
	// ContainerExecCreateTimeout is the timeout for the ContainerExecCreate API.
	ContainerExecCreateTimeout = 1 * time.Minute
	// ContainerExecStartTimeout is the timeout for the ContainerExecStart API.
	ContainerExecStartTimeout = 1 * time.Minute
	// ContainerExecInspectTimeout is the timeout for the ContainerExecInspect API.
	ContainerExecInspectTimeout = 1 * time.Minute
	// StopContainerTimeout is the timeout for the StopContainer API.
	StopContainerTimeout = 30 * time.Second
	// RemoveContainerTimeout is the timeout for the RemoveContainer API.
	RemoveContainerTimeout = 5 * time.Minute

	// CreateVolumeTimeout is the timeout for CreateVolume API.
	CreateVolumeTimeout = 5 * time.Minute
	// InspectVolumeTimeout is the timeout for InspectVolume API.
	InspectVolumeTimeout = 5 * time.Minute
	// RemoveVolumeTimeout is the timeout for RemoveVolume API.
	RemoveVolumeTimeout = 5 * time.Minute

	// ListPluginsTimeout is the timeout for ListPlugins API.
	ListPluginsTimeout = 1 * time.Minute

	// StatsInactivityTimeout controls the amount of time we hold open a
	// connection to the Docker daemon waiting for stats data.
	// We set this a few seconds below our stats publishling interval (20s)
	// so that we can disconnect and warn the user when metrics reporting
	// may be impacted. This is usually caused by a over-stressed docker daemon.
	StatsInactivityTimeout = 18 * time.Second

	// DockerPullBeginTimeout is the timeout from when a 'pull' is called to when
	// we expect to see output on the pull progress stream. This is to work
	// around a docker bug which sometimes results in pulls not progressing.
	DockerPullBeginTimeout = 5 * time.Minute

	// VersionTimeout is the timeout for the Version API
	VersionTimeout = 10 * time.Second

	// InfoTimeout is the timeout for the Info API
	InfoTimeout = 10 * time.Second
)
