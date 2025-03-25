//go:build unit
// +build unit

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

package task

import (
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"

	"github.com/aws/aws-sdk-go-v2/aws"
	acstypes "github.com/aws/aws-sdk-go-v2/service/acs/types"
)

const (
	dockerIDPrefix    = "dockerid-"
	secretKeyWest1    = "/test/secretName_us-west-2"
	secKeyLogDriver   = "/test/secretName1_us-west-1"
	asmSecretKeyWest1 = "arn:aws:secretsmanager:us-west-2:11111:secret:/test/secretName_us-west-2"
	ipv4              = "10.0.0.2"
	ipv4Block         = "/24"
	ipv4Gateway       = "10.0.0.1"
	mac               = "1.2.3.4"
	ipv6              = "f0:234:23"
	ipv6Block         = "/64"
	ignoredUID        = "1337"
	proxyIngressPort  = "15000"
	proxyEgressPort   = "15001"
	appPort           = "9000"
	egressIgnoredIP   = "169.254.169.254"
	testTaskARN       = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
)

var defaultDockerClientAPIVersion = dockerclient.Version_1_17

func strptr(s string) *string { return &s }

func dockerMap(task *Task) map[string]*apicontainer.DockerContainer {
	m := make(map[string]*apicontainer.DockerContainer)
	for _, container := range task.Containers {
		m[container.Name] = &apicontainer.DockerContainer{DockerID: dockerIDPrefix + container.Name, DockerName: "dockername-" + container.Name, Container: container}
	}
	return m
}

func getACSIAMRoleCredentials() *acstypes.IAMRoleCredentials {
	testTime := ttime.Now().Truncate(1 * time.Second).Format(time.RFC3339)
	return &acstypes.IAMRoleCredentials{
		CredentialsId:   strptr("credsId"),
		AccessKeyId:     strptr("keyId"),
		Expiration:      strptr(testTime),
		RoleArn:         strptr("roleArn"),
		SecretAccessKey: strptr("OhhSecret"),
		SessionToken:    strptr("sessionToken"),
	}
}

func getACSEFSTask() *acstypes.Task {
	return &acstypes.Task{
		Arn:           strptr(testTaskARN),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []acstypes.Container{
			{
				Name: strptr("myName1"),
				MountPoints: []acstypes.MountPoint{
					{
						ContainerPath: strptr("/some/path"),
						SourceVolume:  strptr("efsvolume"),
					},
				},
			},
		},
		Volumes: []acstypes.Volume{
			{
				Name: strptr("efsvolume"),
				Type: "efs",
				EfsVolumeConfiguration: &acstypes.EFSVolumeConfiguration{
					AuthorizationConfig: &acstypes.EFSAuthorizationConfig{
						Iam:           "ENABLED",
						AccessPointId: strptr("fsap-123"),
					},
					FileSystemId:          strptr("fs-12345"),
					RootDirectory:         strptr("/tmp"),
					TransitEncryption:     "ENABLED",
					TransitEncryptionPort: aws.Int32(12345),
				},
			},
		},
	}
}

func getEFSTask() *Task {
	return &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "efs-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name:   "efs-volume-test",
				Type:   "efs",
				Volume: getEFSVolumeConfig(),
			},
		},
	}
}

func getEFSVolumeConfig() *taskresourcevolume.EFSVolumeConfig {
	return &taskresourcevolume.EFSVolumeConfig{
		AuthConfig: taskresourcevolume.EFSAuthConfig{
			Iam:           "ENABLED",
			AccessPointId: "fsap-123",
		},
		FileSystemID:          "fs-12345",
		RootDirectory:         "/my/root/dir",
		TransitEncryption:     "ENABLED",
		TransitEncryptionPort: 12345,
	}
}
