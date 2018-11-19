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

package monitor

import (
	"context"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"time"
)

const (
	PrometheusContainerName = "Prometheus_Monitor"
	dockerTimeout           = 2 * time.Minute
)

// Performs the required set-up for the Prometheus container. This function is
// only called if PrometheusMetrics is enabled in the cfg. We perform the
// following steps:
// 1) Create Volume if not created.
// 2) Create Prometheus container if not created.
// 3) Start Prometheus container
func InitPrometheusContainer(ctx context.Context, cfg *config.Config, dg dockerapi.DockerClient, fs ...os.FileSystem) string {
	if !cfg.PrometheusMetricsEnabled {
		return ""
	}
	prometheusVolumeSetup(dg, cfg, ctx)

	containerID := cfg.PrometheusMonitorContainerID

	// We should create the container in 2 situations: if containerID in state file
	// was never initialized (Prometheus container never started) or containerID
	// cannot be inspected. Note the fallthrough keyword here.
	switch {
	case containerID != "":
		_, err := dg.InspectContainer(ctx, containerID, dockerTimeout)
		if err == nil {
			seelog.Infof("Prometheus container found with Docker ID: %s", containerID)
			break
		}
		fallthrough
	case containerID == "":
		seelog.Info("Prometheus container not found. Creating one.")
		// This is so we can mock the FileSystem and optionally pass it into this function
		// for unit testing
		var fsToUse os.FileSystem
		if len(fs) > 0 {
			fsToUse = fs[0]
		} else {
			fsToUse = os.Default
		}
		metadata, err := createPrometheusContainer(dg, cfg, ctx, fsToUse)
		if err != nil {
			seelog.Errorf("Prometheus container could not be created: %s", err.Error())
			return ""
		} else if metadata.Error != nil {
			seelog.Errorf("Prometheus container could not be created: %s", metadata.Error.Error())
			return ""
		}
		containerID = metadata.DockerID
	}
	// We start the Prometheus container after all set-up has been done
	metadata := dg.StartContainer(ctx, containerID, dockerTimeout)
	if metadata.Error == nil {
		seelog.Info("Prometheus container started")
	} else {
		seelog.Errorf("Failed to start Prometheus container: %s", metadata.Error.Error())
		return ""
	}
	return containerID
}

func prometheusVolumeSetup(dg dockerapi.DockerClient, cfg *config.Config, ctx context.Context) {
	resp := dg.InspectVolume(ctx, cfg.PrometheusMonitorVolumeName, dockerTimeout)
	if resp.Error != nil {
		seelog.Infof("Desired Prometheus Volume inspection: %s", resp.Error.Error())
		// Inspection failed, attempt to remove the volume.
		dg.RemoveVolume(ctx, cfg.PrometheusMonitorVolumeName, dockerTimeout)
		// Create the new volume
		dg.CreateVolume(ctx, cfg.PrometheusMonitorVolumeName, "", nil, make(map[string]string), dockerTimeout)
	}
}

func createPrometheusContainer(dg dockerapi.DockerClient, cfg *config.Config, ctx context.Context, fs os.FileSystem) (dockerapi.DockerContainerMetadata, error) {
	// Load the image from the tarball path in the cfg
	_, err := LoadImage(ctx, cfg, dg, fs)
	if err != nil {
		seelog.Errorf("Failed to load Prometheus container image: %s", err.Error())
		return dockerapi.DockerContainerMetadata{}, err
	}
	dockerConfig := dockercontainer.Config{
		Image: cfg.PrometheusMonitorContainerImageName + ":" + cfg.PrometheusMonitorContainerTag,
	}

	// Bind the volume we created to the root prometheus directory in the container
	volumeBinds := make([]string, 1)
	bind := cfg.PrometheusMonitorVolumeName + ":/prometheus"
	volumeBinds[0] = bind

	// The Prometheus Container should always be running and restarted on failure
	restartPolicy := dockercontainer.RestartPolicy{
		Name:              "on-failure",
		MaximumRetryCount: 5,
	}

	// We share the network stack with Host to allow Prometheus to access required
	// ports on the Host
	dockerHostConfig := dockercontainer.HostConfig{
		NetworkMode:   "host",
		Binds:         volumeBinds,
		RestartPolicy: restartPolicy,
	}
	metadata := dg.CreateContainer(ctx, &dockerConfig, &dockerHostConfig, PrometheusContainerName, dockerTimeout)

	return metadata, nil
}
