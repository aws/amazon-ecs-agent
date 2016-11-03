// +build windows,integration

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

package engine

import "github.com/aws/amazon-ecs-agent/agent/api"

const (
	dockerEndpoint  = "npipe:////./pipe/docker_engine"
	testVolumeImage = "amazon/amazon-ecs-volumes-test:make"
)

// TODO implement this
func isDockerRunning() bool { return true }

func createTestContainer() *api.Container {
	return &api.Container{
		Name:          "windows",
		Image:         "microsoft/windowsservercore:latest",
		Command:       []string{},
		Essential:     true,
		DesiredStatus: api.ContainerRunning,
		Cpu:           100,
		Memory:        80,
	}
}

func createTestHostVolumeMountTask(tmpPath string) *api.Task {
	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{api.TaskVolume{Name: "test-tmp", Volume: &api.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{api.MountPoint{ContainerPath: "C:/host/tmp", SourceVolume: "test-tmp"}}
	//`echo -n "hi" > /host/tmp/hello-from-container; if [[ "$(cat /host/tmp/test-file)" != "test-data" ]]; then exit 4; fi; exit 42`}
	testTask.Containers[0].Command = []string{
		`echo "hi" | Out-File -FilePath C:\host\tmp\hello-from-container -Encoding ascii ; $exists = Test-Path C:\host\tmp\test-file ; if (!$exists) { exit 2 } ;$contents = [IO.File]::ReadAllText("C:\host\tmp\test-file") ; if (!$contents -match "test-data") { $contents ; exit 4 } ; exit 42`,
	}
	return testTask
}
