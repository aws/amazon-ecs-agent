// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_secretsmanageriface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/service/secretsmanager"

	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dockerIDPrefix = "dockerid-"

var defaultDockerClientAPIVersion = dockerclient.Version_1_17

func strptr(s string) *string { return &s }

func dockerMap(task *Task) map[string]*apicontainer.DockerContainer {
	m := make(map[string]*apicontainer.DockerContainer)
	for _, container := range task.Containers {
		m[container.Name] = &apicontainer.DockerContainer{DockerID: dockerIDPrefix + container.Name, DockerName: "dockername-" + container.Name, Container: container}
	}
	return m
}

func TestDockerConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name:  "c1",
				Ports: []apicontainer.PortBinding{{10, 10, "", apicontainer.TransportProtocolTCP}, {20, 20, "", apicontainer.TransportProtocolUDP}},
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	_, ok := config.ExposedPorts["10/tcp"]
	if !ok {
		t.Fatal("Could not get exposed ports 10/tcp")
	}
	_, ok = config.ExposedPorts["20/udp"]
	if !ok {
		t.Fatal("Could not get exposed ports 20/udp")
	}
}

func TestDockerConfigCPUShareZero(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  0,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareMinimum(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  1,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 1 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareUnchanged(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  100,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 100 {
		t.Error("CPU shares unexpectedly changed")
	}
}

func TestDockerHostConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name:  "c1",
				Ports: []apicontainer.PortBinding{{10, 10, "", apicontainer.TransportProtocolTCP}, {20, 20, "", apicontainer.TransportProtocolUDP}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)

	bindings, ok := config.PortBindings["10/tcp"]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "10", bindings[0].HostPort, "Wrong hostport")

	bindings, ok = config.PortBindings["20/udp"]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "20", bindings[0].HostPort, "Wrong hostport")
}

func TestDockerHostConfigVolumesFrom(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
			},
			{
				Name:        "c2",
				VolumesFrom: []apicontainer.VolumeFrom{{SourceContainer: "c1"}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)

	if !reflect.DeepEqual(config.VolumesFrom, []string{"dockername-c1"}) {
		t.Error("Expected volumesFrom to be resolved, was: ", config.VolumesFrom)
	}
}

func TestDockerHostConfigRawConfig(t *testing.T) {
	rawHostConfigInput := docker.HostConfig{
		Privileged:     true,
		ReadonlyRootfs: true,
		DNS:            []string{"dns1, dns2"},
		DNSSearch:      []string{"dns.search"},
		ExtraHosts:     []string{"extra:hosts"},
		SecurityOpt:    []string{"foo", "bar"},
		CPUShares:      2,
		LogConfig: docker.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
		Ulimits:          []docker.ULimit{{Name: "ulimit name", Soft: 10, Hard: 100}},
		MemorySwappiness: memorySwappinessDefault,
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, configErr)

	expectedOutput := rawHostConfigInput
	expectedOutput.CPUPercent = minimumCPUPercent
	if runtime.GOOS == "windows" {
		// CPUShares will always be 0 on windows
		expectedOutput.CPUShares = 0
	}
	assertSetStructFieldsEqual(t, expectedOutput, *config)
}

func TestDockerHostConfigPauseContainer(t *testing.T) {
	testTask := &Task{
		ENI: &apieni.ENI{
			ID: "eniID",
		},
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name: "c1",
			},
			&apicontainer.Container{
				Name: PauseContainerName,
				Type: apicontainer.ContainerCNIPause,
			},
		},
	}

	customContainer := testTask.Containers[0]
	pauseContainer := testTask.Containers[1]
	// Verify that the network mode is set to "container:<pause-container-docker-id>"
	// for a non pause container
	config, err := testTask.DockerHostConfig(customContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, "container:"+dockerIDPrefix+PauseContainerName, config.NetworkMode)

	// Verify that the network mode is set to "none" for the pause container
	config, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, networkModeNone, config.NetworkMode)

	// Verify that overridden DNS settings are set for the pause container
	// and not set for non pause containers
	testTask.ENI.DomainNameServers = []string{"169.254.169.253"}
	testTask.ENI.DomainNameSearchList = []string{"us-west-2.compute.internal"}

	// DNS overrides are only applied to the pause container. Verify that the non-pause
	// container contains no overrides
	config, err = testTask.DockerHostConfig(customContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(config.DNS))
	assert.Equal(t, 0, len(config.DNSSearch))

	// Verify DNS settings are overridden for the pause container
	config, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, []string{"169.254.169.253"}, config.DNS)
	assert.Equal(t, []string{"us-west-2.compute.internal"}, config.DNSSearch)

	// Verify eni ExtraHosts  added to HostConfig for pause container
	ipaddr := &apieni.ENIIPV4Address{Primary: true, Address: "10.0.1.1"}
	testTask.ENI.IPV4Addresses = []*apieni.ENIIPV4Address{ipaddr}
	testTask.ENI.PrivateDNSName = "eni.ip.region.compute.internal"

	config, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, []string{"eni.ip.region.compute.internal:10.0.1.1"}, config.ExtraHosts)

	// Verify eni Hostname is added to DockerConfig for pause container
	dockerconfig, dockerConfigErr := testTask.DockerConfig(pauseContainer, defaultDockerClientAPIVersion)
	assert.Nil(t, dockerConfigErr)
	assert.Equal(t, "eni.ip.region.compute.internal", dockerconfig.Hostname)

}

func TestBadDockerHostConfigRawConfig(t *testing.T) {
	for _, badHostConfig := range []string{"malformed", `{"Privileged": "wrongType"}`} {
		testTask := Task{
			Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
			Family:  "myFamily",
			Version: "1",
			Containers: []*apicontainer.Container{
				{
					Name: "c1",
					DockerConfig: apicontainer.DockerConfig{
						HostConfig: strptr(badHostConfig),
					},
				},
			},
		}
		_, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(&testTask), defaultDockerClientAPIVersion)
		assert.Error(t, err)
	}
}

func TestDockerConfigRawConfig(t *testing.T) {
	rawConfigInput := docker.Config{
		Hostname:        "hostname",
		Domainname:      "domainname",
		NetworkDisabled: true,
		DNS:             []string{"dnsfoo", "dnsbar"},
		WorkingDir:      "workdir",
		User:            "user",
	}

	rawConfig, err := json.Marshal(&rawConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if configErr != nil {
		t.Fatal(configErr)
	}

	expectedOutput := rawConfigInput
	expectedOutput.CPUShares = 2

	assertSetStructFieldsEqual(t, expectedOutput, *config)
}

func TestDockerConfigRawConfigNilLabel(t *testing.T) {
	rawConfig, err := json.Marshal(&struct{ Labels map[string]string }{nil})
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	_, configErr := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if configErr != nil {
		t.Fatal(configErr)
	}
}

func TestDockerConfigRawConfigMerging(t *testing.T) {
	// Use a struct that will marshal to the actual message we expect; not
	// docker.Config which will include a lot of zero values.
	rawConfigInput := struct {
		User string `json:"User,omitempty" yaml:"User,omitempty"`
	}{
		User: "user",
	}

	rawConfig, err := json.Marshal(&rawConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				Image:  "image",
				CPU:    50,
				Memory: 1000,
				DockerConfig: apicontainer.DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if configErr != nil {
		t.Fatal(configErr)
	}

	expected := docker.Config{
		Memory:    1000 * 1024 * 1024,
		CPUShares: 50,
		Image:     "image",
		User:      "user",
	}

	assertSetStructFieldsEqual(t, expected, *config)
}

func TestBadDockerConfigRawConfig(t *testing.T) {
	for _, badConfig := range []string{"malformed", `{"Labels": "wrongType"}`} {
		testTask := Task{
			Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
			Family:  "myFamily",
			Version: "1",
			Containers: []*apicontainer.Container{
				{
					Name: "c1",
					DockerConfig: apicontainer.DockerConfig{
						Config: strptr(badConfig),
					},
				},
			},
		}
		_, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
		if err == nil {
			t.Fatal("Expected error, was none for: " + badConfig)
		}
	}
}

func TestGetCredentialsEndpointWhenCredentialsAreSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	credentialsIDInTask := "credsid"
	task := Task{
		Containers: []*apicontainer.Container{
			{
				Name:        "c1",
				Environment: make(map[string]string),
			},
			{
				Name:        "c2",
				Environment: make(map[string]string),
			}},
		credentialsID: credentialsIDInTask,
	}

	taskCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
	}
	credentialsManager.EXPECT().GetTaskCredentials(credentialsIDInTask).Return(taskCredentials, true)
	task.initializeCredentialsEndpoint(credentialsManager)

	// Test if all containers in the task have the environment variable for
	// credentials endpoint set correctly.
	for _, container := range task.Containers {
		env := container.Environment
		_, exists := env[awsSDKCredentialsRelativeURIPathEnvironmentVariableName]
		if !exists {
			t.Errorf("'%s' environment variable not set for container '%s', env: %v", awsSDKCredentialsRelativeURIPathEnvironmentVariableName, container.Name, env)
		}
	}
}

func TestGetCredentialsEndpointWhenCredentialsAreNotSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	task := Task{
		Containers: []*apicontainer.Container{
			{
				Name:        "c1",
				Environment: make(map[string]string),
			},
			{
				Name:        "c2",
				Environment: make(map[string]string),
			}},
	}

	task.initializeCredentialsEndpoint(credentialsManager)

	for _, container := range task.Containers {
		env := container.Environment
		_, exists := env[awsSDKCredentialsRelativeURIPathEnvironmentVariableName]
		if exists {
			t.Errorf("'%s' environment variable should not be set for container '%s'", awsSDKCredentialsRelativeURIPathEnvironmentVariableName, container.Name)
		}
	}
}

func TestPostUnmarshalTaskWithDockerVolumes(t *testing.T) {
	autoprovision := true
	ctrl := gomock.NewController(t)
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{DockerVolume: &docker.Volume{}})
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name: strptr("myName1"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("/some/path"),
						SourceVolume:  strptr("dockervolume"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			{
				Name: strptr("dockervolume"),
				Type: strptr("docker"),
				DockerVolumeConfiguration: &ecsacs.DockerVolumeConfiguration{
					Autoprovision: &autoprovision,
					Scope:         strptr("shared"),
					Driver:        strptr("local"),
					DriverOpts:    make(map[string]*string),
					Labels:        nil,
				},
			},
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 1, len(task.Containers)) // before PostUnmarshalTask
	cfg := config.Config{}
	task.PostUnmarshalTask(&cfg, nil, nil, dockerClient, nil)
	assert.Equal(t, 1, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
	assert.Equal(t, 1, len(task.Volumes), "Should have 1 volume")
	taskVol := task.Volumes[0]
	assert.Equal(t, "dockervolume", taskVol.Name)
	assert.Equal(t, DockerVolumeType, taskVol.Type)
}

func TestPostUnmarshalTaskWithLocalVolumes(t *testing.T) {
	// Constants used here are defined in task_unix_test.go and task_windows_test.go
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name: strptr("myName1"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("/path1/"),
						SourceVolume:  strptr("localvol1"),
					},
				},
			},
			{
				Name: strptr("myName2"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("/path2/"),
						SourceVolume:  strptr("localvol2"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			{
				Name: strptr("localvol1"),
				Type: strptr("host"),
				Host: &ecsacs.HostVolumeProperties{},
			},
			{
				Name: strptr("localvol2"),
				Type: strptr("host"),
				Host: &ecsacs.HostVolumeProperties{},
			},
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 2, len(task.Containers)) // before PostUnmarshalTask
	cfg := config.Config{}
	task.PostUnmarshalTask(&cfg, nil, nil, nil, nil)

	assert.Equal(t, 2, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
	taskResources := task.getResourcesUnsafe()
	assert.Equal(t, 2, len(taskResources), "Should have created 2 volume resources")

	for _, r := range taskResources {
		vol := r.(*taskresourcevolume.VolumeResource)
		assert.Equal(t, "task", vol.VolumeConfig.Scope)
		assert.Equal(t, false, vol.VolumeConfig.Autoprovision)
		assert.Equal(t, "local", vol.VolumeConfig.Driver)
		assert.Equal(t, 0, len(vol.VolumeConfig.DriverOpts))
		assert.Equal(t, 0, len(vol.VolumeConfig.Labels))
	}

}

func TestTaskFromACS(t *testing.T) {
	testTime := ttime.Now().Truncate(1 * time.Second).Format(time.RFC3339)

	intptr := func(i int64) *int64 {
		return &i
	}
	boolptr := func(b bool) *bool {
		return &b
	}
	floatptr := func(f float64) *float64 {
		return &f
	}
	// Testing type conversions, bleh. At least the type conversion itself
	// doesn't look this messy.
	taskFromAcs := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name:        strptr("myName"),
				Cpu:         intptr(10),
				Command:     []*string{strptr("command"), strptr("command2")},
				EntryPoint:  []*string{strptr("sh"), strptr("-c")},
				Environment: map[string]*string{"key": strptr("value")},
				Essential:   boolptr(true),
				Image:       strptr("image:tag"),
				Links:       []*string{strptr("link1"), strptr("link2")},
				Memory:      intptr(100),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("/container/path"),
						ReadOnly:      boolptr(true),
						SourceVolume:  strptr("sourceVolume"),
					},
				},
				Overrides: strptr(`{"command":["a","b","c"]}`),
				PortMappings: []*ecsacs.PortMapping{
					{
						HostPort:      intptr(800),
						ContainerPort: intptr(900),
						Protocol:      strptr("udp"),
					},
				},
				VolumesFrom: []*ecsacs.VolumeFrom{
					{
						ReadOnly:        boolptr(true),
						SourceContainer: strptr("volumeLink"),
					},
				},
				DockerConfig: &ecsacs.DockerConfig{
					Config:     strptr("config json"),
					HostConfig: strptr("hostconfig json"),
					Version:    strptr("version string"),
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			{
				Name: strptr("volName"),
				Type: strptr("host"),
				Host: &ecsacs.HostVolumeProperties{
					SourcePath: strptr("/host/path"),
				},
			},
		},
		RoleCredentials: &ecsacs.IAMRoleCredentials{
			CredentialsId:   strptr("credsId"),
			AccessKeyId:     strptr("keyId"),
			Expiration:      strptr(testTime),
			RoleArn:         strptr("roleArn"),
			SecretAccessKey: strptr("OhhSecret"),
			SessionToken:    strptr("sessionToken"),
		},
		Cpu:    floatptr(2.0),
		Memory: intptr(512),
	}
	expectedTask := &Task{
		Arn:                 "myArn",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Family:              "myFamily",
		Version:             "1",
		Containers: []*apicontainer.Container{
			{
				Name:        "myName",
				Image:       "image:tag",
				Command:     []string{"a", "b", "c"},
				Links:       []string{"link1", "link2"},
				EntryPoint:  &[]string{"sh", "-c"},
				Essential:   true,
				Environment: map[string]string{"key": "value"},
				CPU:         10,
				Memory:      100,
				MountPoints: []apicontainer.MountPoint{
					{
						ContainerPath: "/container/path",
						ReadOnly:      true,
						SourceVolume:  "sourceVolume",
					},
				},
				Overrides: apicontainer.ContainerOverrides{
					Command: &[]string{"a", "b", "c"},
				},
				Ports: []apicontainer.PortBinding{
					{
						HostPort:      800,
						ContainerPort: 900,
						Protocol:      apicontainer.TransportProtocolUDP,
					},
				},
				VolumesFrom: []apicontainer.VolumeFrom{
					{
						ReadOnly:        true,
						SourceContainer: "volumeLink",
					},
				},
				DockerConfig: apicontainer.DockerConfig{
					Config:     strptr("config json"),
					HostConfig: strptr("hostconfig json"),
					Version:    strptr("version string"),
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "volName",
				Type: "host",
				Volume: &taskresourcevolume.FSHostVolume{
					FSSourcePath: "/host/path",
				},
			},
		},
		StartSequenceNumber: 42,
		CPU:                 2.0,
		Memory:              512,
		ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromAcs, &ecsacs.PayloadMessage{SeqNum: &seqNum})

	assert.NoError(t, err)
	assert.EqualValues(t, expectedTask, task)
}

func TestTaskUpdateKnownStatusHappyPath(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskCreated, newStatus, "task status should depend on the earlist container status")
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus(), "task status should depend on the earlist container status")
}

// TestTaskUpdateKnownStatusNotChangeToRunningWithEssentialContainerStopped tests when there is one essential
// container is stopped while the other containers are running, the task status shouldn't be changed to running
func TestTaskUpdateKnownStatusNotChangeToRunningWithEssentialContainerStopped(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskCreated,
		Containers: []*apicontainer.Container{
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskStatusNone, newStatus, "task status should not move to running if essential container is stopped")
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus(), "task status should not move to running if essential container is stopped")
}

// TestTaskUpdateKnownStatusToPendingWithEssentialContainerStopped tests when there is one essential container
// is stopped while other container status are prior to Running, the task status should be updated.
func TestTaskUpdateKnownStatusToPendingWithEssentialContainerStopped(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskCreated, newStatus)
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus())
}

// TestTaskUpdateKnownStatusToPendingWithEssentialContainerStoppedWhenSteadyStateIsResourcesProvisioned
// tests when there is one essential container is stopped while other container status are prior to
// ResourcesProvisioned, the task status should be updated.
func TestTaskUpdateKnownStatusToPendingWithEssentialContainerStoppedWhenSteadyStateIsResourcesProvisioned(t *testing.T) {
	resourcesProvisioned := apicontainerstatus.ContainerResourcesProvisioned
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
				Essential:         true,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				Essential:         true,
			},
			&apicontainer.Container{
				KnownStatusUnsafe:       apicontainerstatus.ContainerCreated,
				Essential:               true,
				SteadyStateStatusUnsafe: &resourcesProvisioned,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskCreated, newStatus)
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus())
}

// TestGetEarliestTaskStatusForContainersEmptyTask verifies that
// `getEarliestKnownTaskStatusForContainers` returns TaskStatusNone when invoked on
// a task with no containers
func TestGetEarliestTaskStatusForContainersEmptyTask(t *testing.T) {
	testTask := &Task{}
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskStatusNone)
}

// TestGetEarliestTaskStatusForContainersWhenKnownStatusIsNotSetForContainers verifies that
// `getEarliestKnownTaskStatusForContainers` returns TaskStatusNone when invoked on
// a task with containers that do not have the `KnownStatusUnsafe` field set
func TestGetEarliestTaskStatusForContainersWhenKnownStatusIsNotSetForContainers(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{},
			&apicontainer.Container{},
		},
	}
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskStatusNone)
}

func TestGetEarliestTaskStatusForContainersWhenSteadyStateIsRunning(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}

	// Since a container is still in CREATED state, the earliest known status
	// for the task based on its container statuses must be `apitaskstatus.TaskCreated`
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskCreated)
	// Ensure that both containers are RUNNING, which means that the earliest known status
	// for the task based on its container statuses must be `apitaskstatus.TaskRunning`
	testTask.Containers[0].SetKnownStatus(apicontainerstatus.ContainerRunning)
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskRunning)
}

func TestGetEarliestTaskStatusForContainersWhenSteadyStateIsResourceProvisioned(t *testing.T) {
	resourcesProvisioned := apicontainerstatus.ContainerResourcesProvisioned
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
			&apicontainer.Container{
				KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
				SteadyStateStatusUnsafe: &resourcesProvisioned,
			},
		},
	}

	// Since a container is still in CREATED state, the earliest known status
	// for the task based on its container statuses must be `apitaskstatus.TaskCreated`
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskCreated)
	testTask.Containers[0].SetKnownStatus(apicontainerstatus.ContainerRunning)
	// Even if all containers transition to RUNNING, the earliest known status
	// for the task based on its container statuses would still be `apitaskstatus.TaskCreated`
	// as one of the containers has RESOURCES_PROVISIONED as its steady state
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskCreated)
	// All of the containers in the task have reached their steady state. Ensure
	// that the earliest known status for the task based on its container states
	// is now `apitaskstatus.TaskRunning`
	testTask.Containers[2].SetKnownStatus(apicontainerstatus.ContainerResourcesProvisioned)
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskRunning)
}

func TestTaskUpdateKnownStatusChecksSteadyStateWhenSetToRunning(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}

	// One of the containers is in CREATED state, expect task to be updated
	// to apitaskstatus.TaskCreated
	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskCreated, newStatus, "Incorrect status returned: %s", newStatus.String())
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus())

	// All of the containers are in RUNNING state, expect task to be updated
	// to apitaskstatus.TaskRunning
	testTask.Containers[0].SetKnownStatus(apicontainerstatus.ContainerRunning)
	newStatus = testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskRunning, newStatus, "Incorrect status returned: %s", newStatus.String())
	assert.Equal(t, apitaskstatus.TaskRunning, testTask.GetKnownStatus())
}

func TestTaskUpdateKnownStatusChecksSteadyStateWhenSetToResourceProvisioned(t *testing.T) {
	resourcesProvisioned := apicontainerstatus.ContainerResourcesProvisioned
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
				Essential:         true,
			},
			&apicontainer.Container{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
				Essential:         true,
			},
			&apicontainer.Container{
				KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
				Essential:               true,
				SteadyStateStatusUnsafe: &resourcesProvisioned,
			},
		},
	}

	// One of the containers is in CREATED state, expect task to be updated
	// to apitaskstatus.TaskCreated
	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskCreated, newStatus, "Incorrect status returned: %s", newStatus.String())
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus())

	// All of the containers are in RUNNING state, but one of the containers
	// has its steady state set to RESOURCES_PROVISIONED, doexpect task to be
	// updated to apitaskstatus.TaskRunning
	testTask.Containers[0].SetKnownStatus(apicontainerstatus.ContainerRunning)
	newStatus = testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskStatusNone, newStatus, "Incorrect status returned: %s", newStatus.String())
	assert.Equal(t, apitaskstatus.TaskCreated, testTask.GetKnownStatus())

	// All of the containers have reached their steady states, expect the task
	// to be updated to `apitaskstatus.TaskRunning`
	testTask.Containers[2].SetKnownStatus(apicontainerstatus.ContainerResourcesProvisioned)
	newStatus = testTask.updateTaskKnownStatus()
	assert.Equal(t, apitaskstatus.TaskRunning, newStatus, "Incorrect status returned: %s", newStatus.String())
	assert.Equal(t, apitaskstatus.TaskRunning, testTask.GetKnownStatus())
}

func assertSetStructFieldsEqual(t *testing.T, expected, actual interface{}) {
	for i := 0; i < reflect.TypeOf(expected).NumField(); i++ {
		expectedValue := reflect.ValueOf(expected).Field(i)
		// All the values we actually expect to see are valid and non-nil
		if !expectedValue.IsValid() || ((expectedValue.Kind() == reflect.Map || expectedValue.Kind() == reflect.Slice) && expectedValue.IsNil()) {
			continue
		}
		expectedVal := expectedValue.Interface()
		actualVal := reflect.ValueOf(actual).Field(i).Interface()
		if !reflect.DeepEqual(expectedVal, actualVal) {
			t.Fatalf("Field %v did not match: %v != %v", reflect.TypeOf(expected).Field(i).Name, expectedVal, actualVal)
		}
	}
}

// TestGetIDErrorPaths performs table tests on GetID with erroneous taskARNs
func TestGetIDErrorPaths(t *testing.T) {
	testCases := []struct {
		arn  string
		name string
	}{
		{"", "EmptyString"},
		{"invalidArn", "InvalidARN"},
		{"arn:aws:ecs:region:account-id:task:task-id", "IncorrectSections"},
		{"arn:aws:ecs:region:account-id:task", "IncorrectResouceSections"},
	}

	task := Task{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task.Arn = tc.arn
			taskID, err := task.GetID()
			assert.Error(t, err, "GetID should return an error")
			assert.Empty(t, taskID, "ID should be empty")
		})
	}
}

// TestGetIDHappyPath validates the happy path of GetID
func TestGetIDHappyPath(t *testing.T) {
	task := Task{
		Arn: "arn:aws:ecs:region:account-id:task/task-id",
	}
	taskID, err := task.GetID()
	assert.NoError(t, err)
	assert.Equal(t, "task-id", taskID)
}

// TestTaskGetENI tests the eni can be correctly acquired by calling GetTaskENI
func TestTaskGetENI(t *testing.T) {
	enisOfTask := &apieni.ENI{
		ID: "id",
	}
	testTask := &Task{
		ENI: enisOfTask,
	}

	eni := testTask.GetTaskENI()
	assert.NotNil(t, eni)
	assert.Equal(t, "id", eni.ID)

	testTask.ENI = nil
	eni = testTask.GetTaskENI()
	assert.Nil(t, eni)
}

// TestTaskFromACSWithOverrides tests the container command is overridden correctly
func TestTaskFromACSWithOverrides(t *testing.T) {
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name: strptr("myName1"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("volumeContainerPath1"),
						SourceVolume:  strptr("volumeName1"),
					},
				},
				Overrides: strptr(`{"command": ["foo", "bar"]}`),
			},
			{
				Name:    strptr("myName2"),
				Command: []*string{strptr("command")},
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("volumeContainerPath2"),
						SourceVolume:  strptr("volumeName2"),
					},
				},
			},
		},
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 2, len(task.Containers)) // before PostUnmarshalTask

	assert.Equal(t, task.Containers[0].Command[0], "foo")
	assert.Equal(t, task.Containers[0].Command[1], "bar")
	assert.Equal(t, task.Containers[1].Command[0], "command")
}

// TestSetPullStartedAt tests the task SetPullStartedAt
func TestSetPullStartedAt(t *testing.T) {
	testTask := &Task{}

	t1 := time.Now()
	t2 := t1.Add(1 * time.Second)

	testTask.SetPullStartedAt(t1)
	assert.Equal(t, t1, testTask.GetPullStartedAt(), "first set of pullStartedAt should succeed")

	testTask.SetPullStartedAt(t2)
	assert.Equal(t, t1, testTask.GetPullStartedAt(), "second set of pullStartedAt should have no impact")
}

// TestSetExecutionStoppedAt tests the task SetExecutionStoppedAt
func TestSetExecutionStoppedAt(t *testing.T) {
	testTask := &Task{}

	t1 := time.Now()
	t2 := t1.Add(1 * time.Second)

	testTask.SetExecutionStoppedAt(t1)
	assert.Equal(t, t1, testTask.GetExecutionStoppedAt(), "first set of executionStoppedAt should succeed")

	testTask.SetExecutionStoppedAt(t2)
	assert.Equal(t, t1, testTask.GetExecutionStoppedAt(), "second set of executionStoppedAt should have no impact")
}

func TestApplyExecutionRoleLogsAuthSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	credentialsIDInTask := "credsid"
	expectedEndpoint := "/v2/credentials/" + credentialsIDInTask

	rawHostConfigInput := docker.HostConfig{
		LogConfig: docker.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	task := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "testFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
		ExecutionCredentialsID: credentialsIDInTask,
	}

	taskCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
	}
	credentialsManager.EXPECT().GetTaskCredentials(credentialsIDInTask).Return(taskCredentials, true)
	task.initializeCredentialsEndpoint(credentialsManager)

	config, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion)
	assert.Nil(t, err)

	err = task.ApplyExecutionRoleLogsAuth(config, credentialsManager)
	assert.Nil(t, err)

	endpoint, ok := config.LogConfig.Config["awslogs-credentials-endpoint"]
	assert.True(t, ok)
	assert.Equal(t, expectedEndpoint, endpoint)
}

func TestApplyExecutionRoleLogsAuthFailEmptyCredentialsID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	rawHostConfigInput := docker.HostConfig{
		LogConfig: docker.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	task := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "testFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
	}

	task.initializeCredentialsEndpoint(credentialsManager)

	config, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion)
	assert.Nil(t, err)

	err = task.ApplyExecutionRoleLogsAuth(config, credentialsManager)
	assert.Error(t, err)
}

func TestApplyExecutionRoleLogsAuthFailNoCredentialsForTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	credentialsIDInTask := "credsid"

	rawHostConfigInput := docker.HostConfig{
		LogConfig: docker.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	task := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "testFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
		ExecutionCredentialsID: credentialsIDInTask,
	}

	credentialsManager.EXPECT().GetTaskCredentials(credentialsIDInTask).Return(credentials.TaskIAMRoleCredentials{}, false)
	task.initializeCredentialsEndpoint(credentialsManager)

	config, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion)
	assert.Error(t, err)

	err = task.ApplyExecutionRoleLogsAuth(config, credentialsManager)
	assert.Error(t, err)
}

// TestSetMinimumMemoryLimit ensures that we set the correct minimum memory limit when the limit is too low
func TestSetMinimumMemoryLimit(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				Memory: uint(1),
			},
		},
	}

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)

	config, cerr := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	assert.Nil(t, cerr)

	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), config.Memory)
	assert.Empty(t, hostconfig.Memory)

	hostconfig, err = testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), dockerclient.Version_1_18)
	assert.Nil(t, err)

	config, cerr = testTask.DockerConfig(testTask.Containers[0], dockerclient.Version_1_18)
	assert.Nil(t, err)
	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), hostconfig.Memory)
	assert.Empty(t, config.Memory)
}

// TestContainerHealthConfig tests that we set the correct container health check config
func TestContainerHealthConfig(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name:            "c1",
				HealthCheckType: apicontainer.DockerHealthCheckType,
				DockerConfig: apicontainer.DockerConfig{
					Config: aws.String(`{
						"HealthCheck":{
							"Test":["command"],
							"Interval":5000000000,
							"Timeout":4000000000,
							"StartPeriod":60000000000,
							"Retries":5}
					}`),
				},
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	require.NotNil(t, config.Healthcheck, "health config was not set in docker config")
	assert.Equal(t, config.Healthcheck.Test, []string{"command"})
	assert.Equal(t, config.Healthcheck.Retries, 5)
	assert.Equal(t, config.Healthcheck.Interval, 5*time.Second)
	assert.Equal(t, config.Healthcheck.Timeout, 4*time.Second)
	assert.Equal(t, config.Healthcheck.StartPeriod, 1*time.Minute)
}

func TestRecordExecutionStoppedAt(t *testing.T) {
	testCases := []struct {
		essential             bool
		status                apicontainerstatus.ContainerStatus
		executionStoppedAtSet bool
		msg                   string
	}{
		{
			essential:             true,
			status:                apicontainerstatus.ContainerStopped,
			executionStoppedAtSet: true,
			msg: "essential container stopped should have executionStoppedAt set",
		},
		{
			essential:             false,
			status:                apicontainerstatus.ContainerStopped,
			executionStoppedAtSet: false,
			msg: "non essential container stopped should not cause executionStoppedAt set",
		},
		{
			essential:             true,
			status:                apicontainerstatus.ContainerRunning,
			executionStoppedAtSet: false,
			msg: "essential non-stop status change should not cause executionStoppedAt set",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Container status: %s, essential: %v, executionStoppedAt should be set: %v", tc.status, tc.essential, tc.executionStoppedAtSet), func(t *testing.T) {
			task := &Task{}
			task.RecordExecutionStoppedAt(&apicontainer.Container{
				Essential:         tc.essential,
				KnownStatusUnsafe: tc.status,
			})
			assert.Equal(t, !tc.executionStoppedAtSet, task.GetExecutionStoppedAt().IsZero(), tc.msg)
		})
	}
}

func TestMarshalUnmarshalTaskASMResource(t *testing.T) {

	expectedCredentialsParameter := "secret-id"
	expectedRegion := "us-west-2"
	expectedExecutionCredentialsID := "credsid"

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				Name:  "myName",
				Image: "image:tag",
				RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
					Type: "asm",
					ASMAuthData: &apicontainer.ASMAuthData{
						CredentialsParameter: expectedCredentialsParameter,
						Region:               expectedRegion,
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	// create asm auth resource
	res := asmauth.NewASMAuthResource(
		task.Arn,
		task.getAllASMAuthDataRequirements(),
		expectedExecutionCredentialsID,
		credentialsManager,
		asmClientCreator)
	res.SetKnownStatus(resourcestatus.ResourceRemoved)

	// add asm auth resource to task
	task.AddResource(asmauth.ResourceName, res)

	// validate asm auth resource
	resource, ok := task.getASMAuthResource()
	assert.True(t, ok)
	asmResource := resource[0].(*asmauth.ASMAuthResource)
	req := asmResource.GetRequiredASMResources()

	assert.Equal(t, resourcestatus.ResourceRemoved, asmResource.GetKnownStatus())
	assert.Equal(t, expectedExecutionCredentialsID, asmResource.GetExecutionCredentialsID())
	assert.Equal(t, expectedCredentialsParameter, req[0].CredentialsParameter)
	assert.Equal(t, expectedRegion, req[0].Region)

	// marshal and unmarshal task
	marshal, err := json.Marshal(task)
	assert.NoError(t, err)

	var otask Task
	err = json.Unmarshal(marshal, &otask)
	assert.NoError(t, err)

	// validate asm auth resource
	oresource, ok := otask.getASMAuthResource()
	assert.True(t, ok)
	oasmResource := oresource[0].(*asmauth.ASMAuthResource)
	oreq := oasmResource.GetRequiredASMResources()

	assert.Equal(t, resourcestatus.ResourceRemoved, oasmResource.GetKnownStatus())
	assert.Equal(t, expectedExecutionCredentialsID, oasmResource.GetExecutionCredentialsID())
	assert.Equal(t, expectedCredentialsParameter, oreq[0].CredentialsParameter)
	assert.Equal(t, expectedRegion, oreq[0].Region)
}

func TestSetTerminalReason(t *testing.T) {

	expectedTerminalReason := "failed to provison resource"
	overrideTerminalReason := "should not override terminal reason"

	task := &Task{}

	// set terminal reason string
	task.SetTerminalReason(expectedTerminalReason)
	assert.Equal(t, expectedTerminalReason, task.GetTerminalReason())

	// try to override terminal reason string, should not overwrite
	task.SetTerminalReason(overrideTerminalReason)
	assert.Equal(t, expectedTerminalReason, task.GetTerminalReason())
}

func TestPopulateASMAuthData(t *testing.T) {
	expectedUsername := "username"
	expectedPassword := "password"

	credentialsParameter := "secret-id"
	region := "us-west-2"

	credentialsID := "execution role"
	accessKeyID := "akid"
	secretAccessKey := "sakid"
	sessionToken := "token"
	executionRoleCredentials := credentials.IAMRoleCredentials{
		CredentialsID:   credentialsID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}

	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
		RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
			Type: "asm",
			ASMAuthData: &apicontainer.ASMAuthData{
				CredentialsParameter: credentialsParameter,
				Region:               region,
			},
		},
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

	// returned asm data
	asmAuthDataBytes, _ := json.Marshal(&asm.AuthDataValue{
		Username: aws.String(expectedUsername),
		Password: aws.String(expectedPassword),
	})
	asmAuthDataVal := string(asmAuthDataBytes)
	asmSecretValue := &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(asmAuthDataVal),
	}

	// create asm auth resource
	asmRes := asmauth.NewASMAuthResource(
		task.Arn,
		task.getAllASMAuthDataRequirements(),
		credentialsID,
		credentialsManager,
		asmClientCreator)

	// add asm auth resource to task
	task.AddResource(asmauth.ResourceName, asmRes)

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(
			credentials.TaskIAMRoleCredentials{
				ARN:                "",
				IAMRoleCredentials: executionRoleCredentials,
			}, true),
		asmClientCreator.EXPECT().NewASMClient(region, executionRoleCredentials).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Return(asmSecretValue, nil),
	)

	// create resource
	err := asmRes.Create()
	require.NoError(t, err)

	err = task.PopulateASMAuthData(container)
	assert.NoError(t, err)

	dac := container.RegistryAuthentication.ASMAuthData.GetDockerAuthConfig()
	assert.Equal(t, expectedUsername, dac.Username)
	assert.Equal(t, expectedPassword, dac.Password)
}

func TestPopulateASMAuthDataNoASMResource(t *testing.T) {

	credentialsParameter := "secret-id"
	region := "us-west-2"

	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
		RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
			Type: "asm",
			ASMAuthData: &apicontainer.ASMAuthData{
				CredentialsParameter: credentialsParameter,
				Region:               region,
			},
		},
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	// asm resource not added to task, call returns error
	err := task.PopulateASMAuthData(container)
	assert.Error(t, err)

}

func TestPopulateASMAuthDataNoDockerAuthConfig(t *testing.T) {
	credentialsParameter := "secret-id"
	region := "us-west-2"
	credentialsID := "execution role"

	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
		RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
			Type: "asm",
			ASMAuthData: &apicontainer.ASMAuthData{
				CredentialsParameter: credentialsParameter,
				Region:               region,
			},
		},
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	//mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

	// create asm auth resource
	asmRes := asmauth.NewASMAuthResource(
		task.Arn,
		task.getAllASMAuthDataRequirements(),
		credentialsID,
		credentialsManager,
		asmClientCreator)

	// add asm auth resource to task
	task.AddResource(asmauth.ResourceName, asmRes)

	// asm resource does not return docker auth config, call returns error
	err := task.PopulateASMAuthData(container)
	assert.Error(t, err)
}

func TestPostUnmarshalTaskASMDockerAuth(t *testing.T) {
	credentialsParameter := "secret-id"
	region := "us-west-2"

	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
		RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
			Type: "asm",
			ASMAuthData: &apicontainer.ASMAuthData{
				CredentialsParameter: credentialsParameter,
				Region:               region,
			},
		},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{}
	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	err := task.PostUnmarshalTask(cfg, credentialsManager, resFields, nil, nil)
	assert.NoError(t, err)
}
