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
	"github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/service/secretsmanager"

	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dockerIDPrefix    = "dockerid-"
	secretKeyWest1    = "/test/secretName_us-west-2"
	secKeyLogDriver   = "/test/secretName1_us-west-1"
	asmSecretKeyWest1 = "arn:aws:secretsmanager:us-west-2:11111:secret:/test/secretName_us-west-2"
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

func TestDockerHostConfigCPUShareZero(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  0,
			},
		},
	}

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}
	if runtime.GOOS == "windows" {
		if hostconfig.CPUShares != 0 {
			// CPUShares will always be 0 on windows
			t.Error("CPU shares expected to be 0 on windows")
		}
	} else if hostconfig.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestDockerHostConfigCPUShareMinimum(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  1,
			},
		},
	}

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	if runtime.GOOS == "windows" {
		if hostconfig.CPUShares != 0 {
			// CPUShares will always be 0 on windows
			t.Error("CPU shares expected to be 0 on windows")
		}
	} else if hostconfig.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestDockerHostConfigCPUShareUnchanged(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  100,
			},
		},
	}

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	if err != nil {
		t.Error(err)
	}

	if runtime.GOOS == "windows" {
		if hostconfig.CPUShares != 0 {
			// CPUShares will always be 0 on windows
			t.Error("CPU shares expected to be 0 on windows")
		}
	} else if hostconfig.CPUShares != 100 {
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
	rawHostConfigInput := dockercontainer.HostConfig{
		Privileged:     true,
		ReadonlyRootfs: true,
		DNS:            []string{"dns1, dns2"},
		DNSSearch:      []string{"dns.search"},
		ExtraHosts:     []string{"extra:hosts"},
		SecurityOpt:    []string{"foo", "bar"},
		Resources: dockercontainer.Resources{
			CPUShares: 2,
			Ulimits:   []*units.Ulimit{{Name: "ulimit name", Soft: 10, Hard: 100}},
		},
		LogConfig: dockercontainer.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
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
			{
				Name: "c1",
			},
			{
				Name: NetworkPauseContainerName,
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
	assert.Equal(t, "container:"+dockerIDPrefix+NetworkPauseContainerName, string(config.NetworkMode))

	// Verify that the network mode is not set to "none"  for the
	// empty volume container
	config, err = testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, networkModeNone, string(config.NetworkMode))

	// Verify that the network mode is set to "none" for the pause container
	config, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.Equal(t, networkModeNone, string(config.NetworkMode))

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
	rawConfigInput := dockercontainer.Config{
		Hostname:        "hostname",
		Domainname:      "domainname",
		NetworkDisabled: true,
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
	// dockercontainer.Config which will include a lot of zero values.
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

	expected := dockercontainer.Config{
		Image: "image",
		User:  "user",
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

func TestGetDockerResources(t *testing.T) {
	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				CPU:    uint(10),
				Memory: uint(256),
			},
		},
	}
	resources := testTask.getDockerResources(testTask.Containers[0])
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(268435456), resources.Memory, "Wrong amount of memory")
}

func TestGetDockerResourcesCPUTooLow(t *testing.T) {
	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				CPU:    uint(0),
				Memory: uint(256),
			},
		},
	}
	resources := testTask.getDockerResources(testTask.Containers[0])
	assert.Equal(t, int64(268435456), resources.Memory, "Wrong amount of memory")

	// Minimum requirement of 2 CPU Shares
	if resources.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestGetDockerResourcesMemoryTooLow(t *testing.T) {
	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				CPU:    uint(10),
				Memory: uint(1),
			},
		},
	}
	resources := testTask.getDockerResources(testTask.Containers[0])
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), resources.Memory,
		"Wrong amount of memory")
}

func TestGetDockerResourcesUnspecifiedMemory(t *testing.T) {
	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				CPU:  uint(10),
			},
		},
	}
	resources := testTask.getDockerResources(testTask.Containers[0])
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(0), resources.Memory, "Wrong amount of memory")
}

func TestPostUnmarshalTaskWithDockerVolumes(t *testing.T) {
	autoprovision := true
	ctrl := gomock.NewController(t)
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{DockerVolume: &types.Volume{}})
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

func TestInitializeContainersV3MetadataEndpoint(t *testing.T) {
	task := Task{
		Containers: []*apicontainer.Container{
			{
				Name:        "c1",
				Environment: make(map[string]string),
			},
		},
	}
	container := task.Containers[0]

	task.initializeContainersV3MetadataEndpoint(utils.NewStaticUUIDProvider("new-uuid"))

	// Test if the v3 endpoint id is set and the endpoint is injected to env
	assert.Equal(t, container.GetV3EndpointID(), "new-uuid")
	assert.Equal(t, container.Environment[apicontainer.MetadataURIEnvironmentVariableName],
		fmt.Sprintf(apicontainer.MetadataURIFormat, "new-uuid"))
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

// Slice of structs for Table Driven testing for sharing PID and IPC resources
var namespaceTests = []struct {
	PIDMode         string
	IPCMode         string
	ShouldProvision bool
}{
	{"", "none", false},
	{"", "", false},
	{"host", "host", false},
	{"task", "task", true},
	{"host", "task", true},
	{"task", "host", true},
	{"", "task", true},
	{"task", "none", true},
	{"host", "none", false},
	{"host", "", false},
	{"", "host", false},
}

// Testing the Getter functions for IPCMode and PIDMode
func TestGetPIDAndIPCFromTask(t *testing.T) {
	for _, aTest := range namespaceTests {
		testTask := &Task{
			Containers: []*apicontainer.Container{
				{
					Name: "c1",
				},
				{
					Name: "c2",
				},
			},
			PIDMode: aTest.PIDMode,
			IPCMode: aTest.IPCMode,
		}
		assert.Equal(t, aTest.PIDMode, testTask.getPIDMode())
		assert.Equal(t, aTest.IPCMode, testTask.getIPCMode())
	}
}

// Tests if NamespacePauseContainer was provisioned in PostUnmarshalTask
func TestPostUnmarshalTaskWithPIDSharing(t *testing.T) {
	for _, aTest := range namespaceTests {
		testTaskFromACS := ecsacs.Task{
			Arn:           strptr("myArn"),
			DesiredStatus: strptr("RUNNING"),
			Family:        strptr("myFamily"),
			PidMode:       strptr(aTest.PIDMode),
			IpcMode:       strptr(aTest.IPCMode),
			Version:       strptr("1"),
			Containers: []*ecsacs.Container{
				{
					Name: strptr("container1"),
				},
				{
					Name: strptr("container2"),
				},
			},
		}

		seqNum := int64(42)
		task, err := TaskFromACS(&testTaskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
		assert.Nil(t, err, "Should be able to handle acs task")
		assert.Equal(t, aTest.PIDMode, task.getPIDMode())
		assert.Equal(t, aTest.IPCMode, task.getIPCMode())
		assert.Equal(t, 2, len(task.Containers)) // before PostUnmarshalTask
		cfg := config.Config{}
		task.PostUnmarshalTask(&cfg, nil, nil, nil, nil)
		if aTest.ShouldProvision {
			assert.Equal(t, 3, len(task.Containers), "Namespace Pause Container should be created.")
		} else {
			assert.Equal(t, 2, len(task.Containers), "Namespace Pause Container should NOT be created.")
		}
	}
}

func TestNamespaceProvisionDependencyAndHostConfig(t *testing.T) {
	for _, aTest := range namespaceTests {
		taskFromACS := ecsacs.Task{
			Arn:           strptr("myArn"),
			DesiredStatus: strptr("RUNNING"),
			Family:        strptr("myFamily"),
			PidMode:       strptr(aTest.PIDMode),
			IpcMode:       strptr(aTest.IPCMode),
			Version:       strptr("1"),
			Containers: []*ecsacs.Container{
				{
					Name: strptr("container1"),
				},
				{
					Name: strptr("container2"),
				},
			},
		}
		seqNum := int64(42)
		task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
		assert.Nil(t, err, "Should be able to handle acs task")
		assert.Equal(t, aTest.PIDMode, task.getPIDMode())
		assert.Equal(t, aTest.IPCMode, task.getIPCMode())
		assert.Equal(t, 2, len(task.Containers)) // before PostUnmarshalTask
		cfg := config.Config{}
		task.PostUnmarshalTask(&cfg, nil, nil, nil, nil)
		if !aTest.ShouldProvision {
			assert.Equal(t, 2, len(task.Containers), "Namespace Pause Container should NOT be created.")
			docMaps := dockerMap(task)
			for _, container := range task.Containers {
				//configure HostConfig for each container
				dockHostCfg, err := task.DockerHostConfig(container, docMaps, defaultDockerClientAPIVersion)
				assert.Nil(t, err)
				assert.Equal(t, task.IPCMode, string(dockHostCfg.IpcMode))
				assert.Equal(t, task.PIDMode, string(dockHostCfg.PidMode))
				switch aTest.IPCMode {
				case "host":
					assert.True(t, dockHostCfg.IpcMode.IsHost())
				case "none":
					assert.True(t, dockHostCfg.IpcMode.IsNone())
				case "":
					assert.True(t, dockHostCfg.IpcMode.IsEmpty())
				}
				switch aTest.PIDMode {
				case "host":
					assert.True(t, dockHostCfg.PidMode.IsHost())
				case "":
					assert.Equal(t, "", string(dockHostCfg.PidMode))
				}
			}
		} else {
			assert.Equal(t, 3, len(task.Containers), "Namespace Pause Container should be created.")

			namespacePause, ok := task.ContainerByName(NamespacePauseContainerName)
			if !ok {
				t.Fatal("Namespace Pause Container not found")
			}

			docMaps := dockerMap(task)
			dockerName := docMaps[namespacePause.Name].DockerID

			for _, container := range task.Containers {
				//configure HostConfig for each container
				dockHostCfg, err := task.DockerHostConfig(container, docMaps, defaultDockerClientAPIVersion)
				assert.Nil(t, err)
				if namespacePause == container {
					// Expected behavior for IPCMode="task" is "shareable"
					if aTest.IPCMode == "task" {
						assert.True(t, dockHostCfg.IpcMode.IsShareable())
					} else {
						assert.True(t, dockHostCfg.IpcMode.IsEmpty())
					}
					// Expected behavior for any PIDMode is ""
					assert.Equal(t, "", string(dockHostCfg.PidMode))
				} else {
					switch aTest.IPCMode {
					case "task":
						assert.True(t, dockHostCfg.IpcMode.IsContainer())
						assert.Equal(t, dockerName, dockHostCfg.IpcMode.Container())
					case "host":
						assert.True(t, dockHostCfg.IpcMode.IsHost())
					}

					switch aTest.PIDMode {
					case "task":
						assert.True(t, dockHostCfg.PidMode.IsContainer())
						assert.Equal(t, dockerName, dockHostCfg.PidMode.Container())
					case "host":
						assert.True(t, dockHostCfg.PidMode.IsHost())
					}
				}
			}
		}
	}
}

func TestAddNamespaceSharingProvisioningDependency(t *testing.T) {
	for _, aTest := range namespaceTests {
		testTask := &Task{
			PIDMode: aTest.PIDMode,
			IPCMode: aTest.IPCMode,
			Containers: []*apicontainer.Container{
				{
					Name:                      "c1",
					TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
				},
				{
					Name:                      "c2",
					TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
				},
			},
		}
		cfg := &config.Config{
			PauseContainerImageName: "pause-container-image-name",
			PauseContainerTag:       "pause-container-tag",
		}
		testTask.addNamespaceSharingProvisioningDependency(cfg)
		if aTest.ShouldProvision {
			pauseContainer, ok := testTask.ContainerByName(NamespacePauseContainerName)
			require.True(t, ok, "Expected to find pause container")
			assert.True(t, pauseContainer.Essential, "Pause Container should be essential")
			assert.Equal(t, len(testTask.Containers), 3)
			for _, cont := range testTask.Containers {
				// Check if dependency to Pause container was set
				if cont.Name == NamespacePauseContainerName {
					continue
				}
				found := false
				for _, contDep := range cont.TransitionDependenciesMap[apicontainerstatus.ContainerPulled].ContainerDependencies {
					if contDep.ContainerName == NamespacePauseContainerName && contDep.SatisfiedStatus == apicontainerstatus.ContainerRunning {
						found = true
					}
				}
				assert.True(t, found, "Dependency should have been found")
			}
		} else {
			assert.Equal(t, len(testTask.Containers), 2, "Pause Container should not have been added")
		}

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
				Secrets: []*ecsacs.Secret{
					{
						Name:      strptr("secret"),
						ValueFrom: strptr("/test/secret"),
						Provider:  strptr("ssm"),
						Region:    strptr("us-west-2"),
					},
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
		Associations: []*ecsacs.Association{
			{
				Containers: []*string{
					strptr("myName"),
				},
				Content: &ecsacs.EncodedString{
					Encoding: strptr("base64"),
					Value:    strptr("val"),
				},
				Name: strptr("gpu1"),
				Type: strptr("gpu"),
			},
			{
				Containers: []*string{
					strptr("myName"),
				},
				Content: &ecsacs.EncodedString{
					Encoding: strptr("base64"),
					Value:    strptr("val"),
				},
				Name: strptr("dev1"),
				Type: strptr("elastic-inference"),
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
				Secrets: []apicontainer.Secret{
					{
						Name:      "secret",
						ValueFrom: "/test/secret",
						Provider:  "ssm",
						Region:    "us-west-2",
					},
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
		Associations: []Association{
			{
				Containers: []string{
					"myName",
				},
				Content: EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "gpu1",
				Type: "gpu",
			},
			{
				Containers: []string{
					"myName",
				},
				Content: EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "dev1",
				Type: "elastic-inference",
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
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				Essential:         true,
			},
			{
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
			{},
			{},
		},
	}
	assert.Equal(t, testTask.getEarliestKnownTaskStatusForContainers(), apitaskstatus.TaskStatusNone)
}

func TestGetEarliestTaskStatusForContainersWhenSteadyStateIsRunning(t *testing.T) {
	testTask := &Task{
		KnownStatusUnsafe: apitaskstatus.TaskStatusNone,
		Containers: []*apicontainer.Container{
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			{
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
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
			{
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
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
			{
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
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerCreated,
				Essential:         true,
			},
			{
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
				Essential:         true,
			},
			{
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

	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
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

	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
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

	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
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

	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), hostconfig.Memory)

	hostconfig, err = testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), dockerclient.Version_1_18)
	assert.Nil(t, err)

	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), hostconfig.Memory)
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
			msg:                   "essential container stopped should have executionStoppedAt set",
		},
		{
			essential:             false,
			status:                apicontainerstatus.ContainerStopped,
			executionStoppedAtSet: false,
			msg:                   "non essential container stopped should not cause executionStoppedAt set",
		},
		{
			essential:             true,
			status:                apicontainerstatus.ContainerRunning,
			executionStoppedAtSet: false,
			msg:                   "essential non-stop status change should not cause executionStoppedAt set",
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

	expectedTerminalReason := "Failed to provision resource"
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

func TestPostUnmarshalTaskSecret(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
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
	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	err := task.PostUnmarshalTask(cfg, credentialsManager, resFields, nil, nil)
	assert.NoError(t, err)
}

func TestPostUnmarshalTaskASMSecret(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
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

	resourceDep := apicontainer.ResourceDependency{
		Name:           asmsecret.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(asmsecret.ASMSecretCreated),
	}

	err := task.PostUnmarshalTask(cfg, credentialsManager, resFields, nil, nil)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(task.ResourcesMapUnsafe))
	assert.Equal(t, resourceDep, task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])
}

func TestGetAllSSMSecretRequirements(t *testing.T) {
	regionWest := "us-west-2"
	regionEast := "us-east-1"

	secret1 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret1",
		Region:    regionWest,
		ValueFrom: "/test/secretName1",
	}

	secret2 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret2",
		Region:    regionWest,
		ValueFrom: "/test/secretName2",
	}

	secret3 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret3",
		Region:    regionEast,
		ValueFrom: "/test/secretName3",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret1, secret2, secret3},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	reqs := task.getAllSSMSecretRequirements()
	assert.Equal(t, secret1, reqs[regionWest][0])
	assert.Equal(t, 1, len(reqs[regionWest]))
}

func TestInitializeAndGetSSMSecretResource(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	task.initializeSSMSecretResource(credentialsManager, resFields)

	resourceDep := apicontainer.ResourceDependency{
		Name:           ssmsecret.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(ssmsecret.SSMSecretCreated),
	}

	assert.Equal(t, resourceDep, task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])
	assert.Equal(t, 0, len(task.Containers[1].TransitionDependenciesMap))

	_, ok := task.getSSMSecretsResource()
	assert.True(t, ok)
}

func TestRequiresSSMSecret(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	assert.Equal(t, true, task.requiresSSMSecret())
}

func TestRequiresSSMSecretNoSecret(t *testing.T) {
	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	assert.Equal(t, false, task.requiresSSMSecret())
}

func TestRequiresASMSecret(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	assert.True(t, task.requiresASMSecret())
}

func TestRequiresASMSecretNoSecret(t *testing.T) {
	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	assert.False(t, task.requiresASMSecret())
}

func TestGetAllASMSecretRequirements(t *testing.T) {
	regionWest := "us-west-2"
	regionEast := "us-east-1"

	secret1 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret1",
		Region:    regionWest,
		ValueFrom: "/test/secretName1",
	}

	secret2 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret2",
		Region:    regionWest,
		ValueFrom: "/test/secretName2",
	}

	secret3 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret3",
		Region:    regionEast,
		ValueFrom: "/test/secretName3",
	}

	secret4 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret4",
		Region:    regionWest,
		ValueFrom: "/test/secretName1",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret1, secret2, secret3, secret4},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	reqs := task.getAllASMSecretRequirements()
	assert.Equal(t, secret1, reqs["/test/secretName1_us-west-2"])
	assert.Equal(t, secret2, reqs["/test/secretName2_us-west-2"])
	assert.Equal(t, 2, len(reqs))
}

func TestInitializeAndGetASMSecretResource(t *testing.T) {
	secret := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret",
		Region:    "us-west-2",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	container1 := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   nil,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	task.initializeASMSecretResource(credentialsManager, resFields)

	resourceDep := apicontainer.ResourceDependency{
		Name:           asmsecret.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(asmsecret.ASMSecretCreated),
	}

	assert.Equal(t, resourceDep, task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])
	assert.Equal(t, 0, len(task.Containers[1].TransitionDependenciesMap))

	_, ok := task.getASMSecretsResource()
	assert.True(t, ok)
}

func TestPopulateSecrets(t *testing.T) {
	secret1 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret1",
		Region:    "us-west-2",
		Type:      "ENVIRONMENT_VARIABLE",
		ValueFrom: "/test/secretName",
	}

	secret2 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret2",
		Region:    "us-west-2",
		Type:      "ENVIRONMENT_VARIABLE",
		ValueFrom: "arn:aws:secretsmanager:us-west-2:11111:secret:/test/secretName",
	}

	secret3 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "splunk-token",
		Region:    "us-west-1",
		Target:    "LOG_DRIVER",
		ValueFrom: "/test/secretName1",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret1, secret2, secret3},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	hostConfig := &dockercontainer.HostConfig{}
	logDriverName := "splunk"
	hostConfig.LogConfig.Type = logDriverName
	configMap := map[string]string{}
	hostConfig.LogConfig.Config = configMap

	ssmRes := &ssmsecret.SSMSecretResource{}
	ssmRes.SetCachedSecretValue(secretKeyWest1, "secretValue1")

	asmRes := &asmsecret.ASMSecretResource{}
	asmRes.SetCachedSecretValue(asmSecretKeyWest1, "secretValue2")

	ssmRes.SetCachedSecretValue(secKeyLogDriver, "secretValue3")

	task.AddResource(ssmsecret.ResourceName, ssmRes)
	task.AddResource(asmsecret.ResourceName, asmRes)

	task.PopulateSecrets(hostConfig, container)
	assert.Equal(t, "secretValue1", container.Environment["secret1"])
	assert.Equal(t, "secretValue2", container.Environment["secret2"])
	assert.Equal(t, "", container.Environment["secret3"])
	assert.Equal(t, "secretValue3", hostConfig.LogConfig.Config["splunk-token"])
}

func TestPopulateSecretsAsEnvOnlySSM(t *testing.T) {
	secret1 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret1",
		Region:    "us-west-2",
		Type:      "MOUNT_POINT",
		ValueFrom: "arn:aws:secretsmanager:us-west-2:11111:secret:/test/secretName",
	}

	secret2 := apicontainer.Secret{
		Provider:  "asm",
		Name:      "secret2",
		Region:    "us-west-1",
		ValueFrom: "/test/secretName1",
		Target:    "LOG_DRIVER",
	}

	secret3 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "secret3",
		Region:    "us-west-2",
		Type:      "ENVIRONMENT_VARIABLE",
		ValueFrom: "/test/secretName",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret1, secret2, secret3},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	asmRes := &asmsecret.ASMSecretResource{}
	asmRes.SetCachedSecretValue(asmSecretKeyWest1, "secretValue1")
	asmRes.SetCachedSecretValue(secKeyLogDriver, "secretValue2")

	ssmRes := &ssmsecret.SSMSecretResource{}
	ssmRes.SetCachedSecretValue(secretKeyWest1, "secretValue3")

	task.AddResource(ssmsecret.ResourceName, ssmRes)
	task.AddResource(asmsecret.ResourceName, asmRes)

	hostConfig := &dockercontainer.HostConfig{}

	task.PopulateSecrets(hostConfig, container)

	assert.Equal(t, "secretValue3", container.Environment["secret3"])
	assert.Equal(t, 1, len(container.Environment))
}

func TestAddGPUResource(t *testing.T) {
	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
	}

	container1 := &apicontainer.Container{
		Name:  "myName1",
		Image: "image:tag",
	}

	association := []Association{
		{
			Containers: []string{
				"myName",
			},
			Content: EncodedString{
				Encoding: "base64",
				Value:    "val",
			},
			Name: "gpu1",
			Type: "gpu",
		},
		{
			Containers: []string{
				"myName",
			},
			Content: EncodedString{
				Encoding: "base64",
				Value:    "val",
			},
			Name: "gpu2",
			Type: "gpu",
		},
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
		Associations:       association,
	}

	err := task.addGPUResource()

	assert.Equal(t, []string{"gpu1", "gpu2"}, container.GPUIDs)
	assert.Equal(t, []string(nil), container1.GPUIDs)
	assert.NoError(t, err)
}

func TestAddGPUResourceWithInvalidContainer(t *testing.T) {
	container := &apicontainer.Container{
		Name:  "myName",
		Image: "image:tag",
	}

	association := []Association{
		{
			Containers: []string{
				"myName1",
			},
			Content: EncodedString{
				Encoding: "base64",
				Value:    "val",
			},
			Name: "gpu1",
			Type: "gpu",
		},
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
		Associations:       association,
	}
	err := task.addGPUResource()
	assert.Error(t, err)
}

func TestPopulateGPUEnvironmentVariables(t *testing.T) {
	container := &apicontainer.Container{
		Name:   "myName",
		Image:  "image:tag",
		GPUIDs: []string{"gpu1", "gpu2"},
	}

	container1 := &apicontainer.Container{
		Name:  "myName1",
		Image: "image:tag",
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container, container1},
	}

	task.populateGPUEnvironmentVariables()

	environment := make(map[string]string)
	environment[NvidiaVisibleDevicesEnvVar] = "gpu1,gpu2"

	assert.Equal(t, environment, container.Environment)
	assert.Equal(t, map[string]string(nil), container1.Environment)
}

func TestDockerHostConfigNvidiaRuntime(t *testing.T) {
	testTask := &Task{
		Arn: "test",
		Containers: []*apicontainer.Container{
			{
				Name:   "myName1",
				Image:  "image:tag",
				GPUIDs: []string{"gpu1"},
			},
		},
		Associations: []Association{
			{
				Containers: []string{
					"myName1",
				},
				Content: EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "gpu1",
				Type: "gpu",
			},
		},
		NvidiaRuntime: config.DefaultNvidiaRuntime,
	}

	testTask.addGPUResource()
	dockerHostConfig, _ := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Equal(t, testTask.NvidiaRuntime, dockerHostConfig.Runtime)
}

func TestDockerHostConfigRuntimeWithoutGPU(t *testing.T) {
	testTask := &Task{
		Arn: "test",
		Containers: []*apicontainer.Container{
			{
				Name:  "myName1",
				Image: "image:tag",
			},
		},
	}

	testTask.addGPUResource()
	dockerHostConfig, _ := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Equal(t, "", dockerHostConfig.Runtime)
}

func TestDockerHostConfigNoNvidiaRuntime(t *testing.T) {
	testTask := &Task{
		Arn: "test",
		Containers: []*apicontainer.Container{
			{
				Name:   "myName1",
				Image:  "image:tag",
				GPUIDs: []string{"gpu1"},
			},
		},
		Associations: []Association{
			{
				Containers: []string{
					"myName1",
				},
				Content: EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "gpu1",
				Type: "gpu",
			},
		},
	}

	testTask.addGPUResource()
	_, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion)
	assert.Error(t, err)
}

func TestAssociationsByTypeAndContainer(t *testing.T) {
	associationType := "elastic-inference"
	container1 := &apicontainer.Container{
		Name: "containerName1",
	}
	container2 := &apicontainer.Container{
		Name: "containerName2",
	}
	association1 := Association{
		Containers: []string{container1.Name},
		Type:       associationType,
		Name:       "dev1",
	}
	association2 := Association{
		Containers: []string{container1.Name, container2.Name},
		Type:       associationType,
		Name:       "dev2",
	}
	task := &Task{
		Associations: []Association{association1, association2},
	}

	// container 1 is associated with association 1 and association 2
	assert.Equal(t, task.AssociationsByTypeAndContainer(associationType, container1.Name),
		[]string{association1.Name, association2.Name})
	// container 2 is associated with association 2
	assert.Equal(t, task.AssociationsByTypeAndContainer(associationType, container2.Name),
		[]string{association2.Name})
}

func TestAssociationByTypeAndName(t *testing.T) {
	association1 := Association{
		Type: "elastic-inference",
		Name: "dev1",
	}
	association2 := Association{
		Type: "other-type",
		Name: "dev2",
	}
	task := &Task{
		Associations: []Association{association1, association2},
	}

	// positive cases
	association, ok := task.AssociationByTypeAndName("elastic-inference", "dev1")
	assert.Equal(t, *association, association1)
	association, ok = task.AssociationByTypeAndName("other-type", "dev2")
	assert.Equal(t, *association, association2)

	// negative cases
	association, ok = task.AssociationByTypeAndName("elastic-inference", "dev2")
	assert.False(t, ok)
	association, ok = task.AssociationByTypeAndName("other-type", "dev1")
	assert.False(t, ok)
}

func TestTaskGPUEnabled(t *testing.T) {
	testTask := &Task{
		Associations: []Association{
			{
				Containers: []string{
					"myName1",
				},
				Content: EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "gpu1",
				Type: "gpu",
			},
		},
	}

	assert.True(t, testTask.isGPUEnabled())
}

func TestTaskGPUDisabled(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "myName1",
			},
		},
	}
	assert.False(t, testTask.isGPUEnabled())
}

func TestInitializeContainerOrderingWithLinksAndVolumesFrom(t *testing.T) {
	containerWithOnlyVolume := &apicontainer.Container{
		Name:        "myName",
		Image:       "image:tag",
		VolumesFrom: []apicontainer.VolumeFrom{{SourceContainer: "myName1"}},
	}

	containerWithOnlyLink := &apicontainer.Container{
		Name:  "myName1",
		Image: "image:tag",
		Links: []string{"myName"},
	}

	containerWithBothVolumeAndLink := &apicontainer.Container{
		Name:        "myName2",
		Image:       "image:tag",
		VolumesFrom: []apicontainer.VolumeFrom{{SourceContainer: "myName"}},
		Links:       []string{"myName1"},
	}

	containerWithNoVolumeOrLink := &apicontainer.Container{
		Name:  "myName3",
		Image: "image:tag",
	}

	task := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{containerWithOnlyVolume, containerWithOnlyLink,
			containerWithBothVolumeAndLink, containerWithNoVolumeOrLink},
	}

	err := task.initializeContainerOrderingForVolumes()
	assert.NoError(t, err)
	err = task.initializeContainerOrderingForLinks()
	assert.NoError(t, err)

	containerResultWithVolume := task.Containers[0]
	assert.Equal(t, "myName1", containerResultWithVolume.DependsOn[0].ContainerName)
	assert.Equal(t, ContainerOrderingCreateCondition, containerResultWithVolume.DependsOn[0].Condition)

	containerResultWithLink := task.Containers[1]
	assert.Equal(t, "myName", containerResultWithLink.DependsOn[0].ContainerName)
	assert.Equal(t, ContainerOrderingStartCondition, containerResultWithLink.DependsOn[0].Condition)

	containerResultWithBothVolumeAndLink := task.Containers[2]
	assert.Equal(t, "myName", containerResultWithBothVolumeAndLink.DependsOn[0].ContainerName)
	assert.Equal(t, ContainerOrderingCreateCondition, containerResultWithBothVolumeAndLink.DependsOn[0].Condition)
	assert.Equal(t, "myName1", containerResultWithBothVolumeAndLink.DependsOn[1].ContainerName)
	assert.Equal(t, ContainerOrderingStartCondition, containerResultWithBothVolumeAndLink.DependsOn[1].Condition)

	containerResultWithNoVolumeOrLink := task.Containers[3]
	assert.Equal(t, 0, len(containerResultWithNoVolumeOrLink.DependsOn))
}

func TestInitializeContainerOrderingWithError(t *testing.T) {
	containerWithVolumeError := &apicontainer.Container{
		Name:        "myName",
		Image:       "image:tag",
		VolumesFrom: []apicontainer.VolumeFrom{{SourceContainer: "dummyContainer"}},
	}

	containerWithLinkError1 := &apicontainer.Container{
		Name:  "myName1",
		Image: "image:tag",
		Links: []string{"dummyContainer"},
	}

	containerWithLinkError2 := &apicontainer.Container{
		Name:  "myName2",
		Image: "image:tag",
		Links: []string{"myName:link1:link2"},
	}

	task1 := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{containerWithVolumeError, containerWithLinkError1},
	}

	task2 := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{containerWithVolumeError, containerWithLinkError2},
	}

	errVolume1 := task1.initializeContainerOrderingForVolumes()
	assert.Error(t, errVolume1)
	errLink1 := task1.initializeContainerOrderingForLinks()
	assert.Error(t, errLink1)

	errVolume2 := task2.initializeContainerOrderingForVolumes()
	assert.Error(t, errVolume2)
	errLink2 := task2.initializeContainerOrderingForLinks()
	assert.Error(t, errLink2)
}

func TestTaskFromACSPerContainerTimeouts(t *testing.T) {
	modelTimeout := int64(10)
	expectedTimeout := uint(modelTimeout)

	taskFromACS := ecsacs.Task{
		Containers: []*ecsacs.Container{
			{
				StartTimeout: aws.Int64(modelTimeout),
				StopTimeout:  aws.Int64(modelTimeout),
			},
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")

	assert.Equal(t, task.Containers[0].StartTimeout, expectedTimeout)
	assert.Equal(t, task.Containers[0].StopTimeout, expectedTimeout)
}
