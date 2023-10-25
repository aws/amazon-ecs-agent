//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package stats

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"

	"github.com/aws/aws-sdk-go/aws"
)

func (engine *DockerStatsEngine) getEBSVolumeMetrics(taskArn string) []*ecstcs.VolumeMetric {
	task, err := engine.resolver.ResolveTaskByARN(taskArn)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to get corresponding task from dd with task arn: %s", taskArn))
		return nil
	}

	if !task.IsEBSTaskAttachEnabled() {
		logger.Debug("Task not EBS-backed, skip gathering EBS volume metrics.", logger.Fields{
			"taskArn": taskArn,
		})
		return nil
	}

	// TODO: Remove the CSI client from the stats engine and just always have the CSI client created
	// since a new connection is created regardless and it'll make the stats engine less stateful
	if engine.csiClient == nil {
		client := csiclient.NewCSIClient(filepath.Join(csiclient.DefaultSocketHostPath, csiclient.DefaultImageName, csiclient.DefaultSocketName))
		engine.csiClient = &client
	}
	return engine.fetchEBSVolumeMetrics(task, taskArn)
}

func (engine *DockerStatsEngine) fetchEBSVolumeMetrics(task *apitask.Task, taskArn string) []*ecstcs.VolumeMetric {
	var metrics []*ecstcs.VolumeMetric
	for _, tv := range task.Volumes {
		if tv.Volume.GetType() == taskresourcevolume.EBSVolumeType {
			volumeId := tv.Volume.GetVolumeId()
			hostPath := tv.Volume.Source()
			volumeName := tv.Volume.GetVolumeName()
			metric, err := engine.getVolumeMetricsWithTimeout(volumeId, hostPath)
			if err != nil {
				logger.Error("Failed to gather metrics for EBS volume", logger.Fields{
					"VolumeId":             volumeId,
					"SourceVolumeHostPath": hostPath,
					"Error":                err,
				})
				continue
			}
			usedBytes := aws.Float64((float64)(metric.Used))
			totalBytes := aws.Float64((float64)(metric.Capacity))
			metrics = append(metrics, &ecstcs.VolumeMetric{
				VolumeId:   aws.String(volumeId),
				VolumeName: aws.String(volumeName),
				Utilized: &ecstcs.UDoubleCWStatsSet{
					Max:         usedBytes,
					Min:         usedBytes,
					SampleCount: aws.Int64(1),
					Sum:         usedBytes,
				},
				Size: &ecstcs.UDoubleCWStatsSet{
					Max:         totalBytes,
					Min:         totalBytes,
					SampleCount: aws.Int64(1),
					Sum:         totalBytes,
				},
			})
		}
	}
	return metrics
}

func (engine *DockerStatsEngine) getVolumeMetricsWithTimeout(volumeId, hostPath string) (*csiclient.Metrics, error) {
	derivedCtx, cancel := context.WithTimeout(engine.ctx, time.Second*1)
	// releases resources if GetVolumeMetrics finishes before timeout
	defer cancel()
	return engine.csiClient.GetVolumeMetrics(derivedCtx, volumeId, hostPath)
}
