// +build windows

// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/stretchr/testify/assert"
)

const (
	emptyVolumeName1                  = "Empty-Volume-1"
	emptyVolumeContainerPath1         = `C:\my\empty-volume-1`
	expectedEmptyVolumeGeneratedPath1 = `c:\ecs-empty-volume\empty-volume-1`

	emptyVolumeName2                  = "empty-volume-2"
	emptyVolumeContainerPath2         = `C:\my\empty-volume-2`
	expectedEmptyVolumeGeneratedPath2 = `c:\ecs-empty-volume\` + emptyVolumeName2

	expectedEmptyVolumeContainerImage = "microsoft/windowsservercore"
	expectedEmptyVolumeContainerTag   = "latest"
	expectedEmptyVoluemContainerCmd   = "not-applicable"
)

func TestPostUnmarshalWindowsCanonicalPaths(t *testing.T) {
	// Testing type conversions, bleh. At least the type conversion itself
	// doesn't look this messy.
	taskFromAcs := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			&ecsacs.Container{
				Name: strptr("myName"),
				MountPoints: []*ecsacs.MountPoint{
					&ecsacs.MountPoint{
						ContainerPath: strptr(`C:/Container/Path`),
						SourceVolume:  strptr("sourceVolume"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			&ecsacs.Volume{
				Name: strptr("sourceVolume"),
				Host: &ecsacs.HostVolumeProperties{
					SourcePath: strptr(`C:/Host/path`),
				},
			},
		},
	}
	expectedTask := &Task{
		Arn:           "myArn",
		DesiredStatus: TaskRunning,
		Family:        "myFamily",
		Version:       "1",
		Containers: []*Container{
			&Container{
				Name: "myName",
				MountPoints: []MountPoint{
					MountPoint{
						ContainerPath: `c:\container\path`,
						SourceVolume:  "sourceVolume",
					},
				},
			},
		},
		Volumes: []TaskVolume{
			TaskVolume{
				Name: "sourceVolume",
				Volume: &FSHostVolume{
					FSSourcePath: `c:\host\path`,
				},
			},
		},
		StartSequenceNumber: 42,
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromAcs, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	task.PostUnmarshalTask(nil)

	assert.Equal(t, expectedTask.Containers, task.Containers, "Containers should be equal")
	assert.Equal(t, expectedTask.Volumes, task.Volumes, "Volumes should be equal")
}
