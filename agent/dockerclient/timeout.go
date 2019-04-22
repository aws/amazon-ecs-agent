// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	// PullImageTimeout is the timeout for the PullImage API
	PullImageTimeout = 2 * time.Hour
	// ListImagesTimeout is the timeout for the ListImages API
	ListImagesTimeout = 10 * time.Minute
	// LoadImageTimeout is the timeout for the LoadImage API. It's set
	// to much lower value than pullImageTimeout as it involves loading
	// image from either a file or STDIN
	// calls involved.
	// TODO: Benchmark and re-evaluate this value
	LoadImageTimeout = 10 * time.Minute
	// RemoveImageTimeout is the timeout for the RemoveImage API.
	RemoveImageTimeout = 3 * time.Minute

	// CreateContainerTimeout is the timeout for the CreateContainer API.
	CreateContainerTimeout = 4 * time.Minute
	// ListContainersTimeout is the timeout for the ListContainers API.
	ListContainersTimeout = 10 * time.Minute
	// InspectContainerTimeout is the timeout for the InspectContainer API.
	InspectContainerTimeout = 30 * time.Second
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
	// connection to the Docker daemon waiting for stats data
	StatsInactivityTimeout = 5 * time.Second

	// DockerPullBeginTimeout is the timeout from when a 'pull' is called to when
	// we expect to see output on the pull progress stream. This is to work
	// around a docker bug which sometimes results in pulls not progressing.
	DockerPullBeginTimeout = 5 * time.Minute

	// VersionTimeout is the timeout for the Version API
	VersionTimeout = 10 * time.Second
)
