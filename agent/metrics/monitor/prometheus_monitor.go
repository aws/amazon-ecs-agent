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
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"time"
)

const (
	PrometheusContainerName = "Prometheus_Monitor"
)

// Performs the required set-up for the Prometheus container. This function is
// only called if PrometheusMetrics is enabled in the cfg. We perform the
// following steps:
// 1) Create Volume if not created.
// 2) Create Prometheus container if not created.
// 3) Start Prometheus container
func InitPrometheusContainer(ctx context.Context, cfg *config.Config, dg dockerapi.DockerClient) string {
	if !cfg.PrometheusMetricsEnabled {
		return ""
	}
	PrometheusVolumeSetup(dg, cfg, ctx)

	containerID := cfg.PrometheusMonitorContainerID

	// We should create the container in 2 situations: if containerID in state file
	// was never initialized (Prometheus container never started) or containerID
	// cannot be inspected. Note the fallthrough keyword here.
	switch {
	case containerID != "":
		_, err := dg.InspectContainer(ctx, containerID, 1*time.Minute)
		if err == nil {
			seelog.Infof("Prometheus container found with Docker ID: %s", containerID)
			break
		}
		fallthrough
	case containerID == "":
		seelog.Info("Prometheus container not found. Creating one.")
		metadata := CreatePrometheusContainer(dg, cfg, ctx)
		containerID = metadata.DockerID
	}
	// We start the Prometheus container after all set-up has been done
	metadata := dg.StartContainer(ctx, containerID, 2*time.Minute)
	if metadata.Error == nil {
		seelog.Info("Prometheus container started")
	} else {
		seelog.Errorf("Failed to start Prometheus container: %s", metadata.Error.Error())
		return ""
	}
	return containerID
}

func PrometheusVolumeSetup(dg dockerapi.DockerClient, cfg *config.Config, ctx context.Context) {
	resp := dg.InspectVolume(ctx, cfg.PrometheusMonitorVolumeName, 1*time.Minute)
	if resp.Error != nil {
		// Inspection failed, attempt to remove the volume.
		dg.RemoveVolume(ctx, cfg.PrometheusMonitorVolumeName, 1*time.Minute)
		// Create the new volume
		resp = dg.CreateVolume(ctx, cfg.PrometheusMonitorVolumeName, "", nil, make(map[string]string), 2*time.Minute)
	}
}

func CreatePrometheusContainer(dg dockerapi.DockerClient, cfg *config.Config, ctx context.Context) dockerapi.DockerContainerMetadata {
	// Load the image from the tarball path in the cfg
	LoadImage(ctx, cfg, dg)
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
	metadata := dg.CreateContainer(ctx, &dockerConfig, &dockerHostConfig, PrometheusContainerName, 2*time.Minute)

	return metadata
}
