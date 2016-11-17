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
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func strptr(s string) *string { return &s }

func dockerMap(task *Task) map[string]*DockerContainer {
	m := make(map[string]*DockerContainer)
	for _, c := range task.Containers {
		m[c.Name] = &DockerContainer{DockerId: "dockerid-" + c.Name, DockerName: "dockername-" + c.Name, Container: c}
	}
	return m
}

func TestTaskOverridden(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}},
			},
		},
	}

	overridden := testTask.Overridden()
	if overridden.Containers[0] == testTask.Containers[0] {
		t.Error("Containers were pointer-equal, not overridden")
	}
}

func TestDockerConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}, PortBinding{20, 20, "", TransportProtocolUDP}},
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
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
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  0,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareMinimum(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  1,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 1 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareUnchanged(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  100,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 100 {
		t.Error("CPU shares unexpectedly changed")
	}
}

func TestDockerHostConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}, PortBinding{20, 20, "", TransportProtocolUDP}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask))
	if err != nil {
		t.Error(err)
	}

	bindings, ok := config.PortBindings["10/tcp"]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "10", bindings[0].HostPort, "Wrong hostport")
	assert.Equal(t, portBindingHostIP, bindings[0].HostIP, "Wrong hostIP")

	bindings, ok = config.PortBindings["20/udp"]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "20", bindings[0].HostPort, "Wrong hostport")
	assert.Equal(t, portBindingHostIP, bindings[0].HostIP, "Wrong hostIP")
}

func TestDockerHostConfigVolumesFrom(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
			},
			&Container{
				Name:        "c2",
				VolumesFrom: []VolumeFrom{VolumeFrom{SourceContainer: "c1"}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask))
	if err != nil {
		t.Fatal("Error creating config: ", err)
	}
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
		LogConfig: docker.LogConfig{
			Type:   "foo",
			Config: map[string]string{"foo": "bar"},
		},
		Ulimits: []docker.ULimit{docker.ULimit{Name: "ulimit name", Soft: 10, Hard: 100}},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*Container{
			&Container{
				Name: "c1",
				DockerConfig: DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask))
	if configErr != nil {
		t.Fatal(configErr)
	}

	expectedOutput := rawHostConfigInput

	assertSetStructFieldsEqual(t, expectedOutput, *config)
}

func TestDockerHostConfigRawConfigMerging(t *testing.T) {
	// Use a struct that will marshal to the actual message we expect; not
	// docker.HostConfig which will include a lot of zero values.
	rawHostConfigInput := struct {
		Privileged  bool     `json:"Privileged,omitempty" yaml:"Privileged,omitempty"`
		SecurityOpt []string `json:"SecurityOpt,omitempty" yaml:"SecurityOpt,omitempty"`
	}{
		Privileged:  true,
		SecurityOpt: []string{"foo", "bar"},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*Container{
			&Container{
				Name:        "c1",
				Image:       "image",
				Cpu:         50,
				Memory:      100,
				VolumesFrom: []VolumeFrom{VolumeFrom{SourceContainer: "c2"}},
				DockerConfig: DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
			&Container{
				Name: "c2",
			},
		},
	}

	hostConfig, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask))
	if configErr != nil {
		t.Fatal(configErr)
	}

	expected := docker.HostConfig{
		Privileged:  true,
		SecurityOpt: []string{"foo", "bar"},
		VolumesFrom: []string{"dockername-c2"},
	}

	assertSetStructFieldsEqual(t, expected, *hostConfig)
}

func TestBadDockerHostConfigRawConfig(t *testing.T) {
	for _, badHostConfig := range []string{"malformed", `{"Privileged": "wrongType"}`} {
		testTask := Task{
			Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
			Family:  "myFamily",
			Version: "1",
			Containers: []*Container{
				&Container{
					Name: "c1",
					DockerConfig: DockerConfig{
						HostConfig: strptr(badHostConfig),
					},
				},
			},
		}
		_, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(&testTask))
		if err == nil {
			t.Fatal("Expected error, was none for: " + badHostConfig)
		}
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
		Containers: []*Container{
			&Container{
				Name: "c1",
				DockerConfig: DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerConfig(testTask.Containers[0])
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
		Containers: []*Container{
			&Container{
				Name: "c1",
				DockerConfig: DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	_, configErr := testTask.DockerConfig(testTask.Containers[0])
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
		Containers: []*Container{
			&Container{
				Name:   "c1",
				Image:  "image",
				Cpu:    50,
				Memory: 100,
				DockerConfig: DockerConfig{
					Config: strptr(string(rawConfig)),
				},
			},
		},
	}

	config, configErr := testTask.DockerConfig(testTask.Containers[0])
	if configErr != nil {
		t.Fatal(configErr)
	}

	expected := docker.Config{
		Memory:    100 * 1024 * 1024,
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
			Containers: []*Container{
				&Container{
					Name: "c1",
					DockerConfig: DockerConfig{
						Config: strptr(badConfig),
					},
				},
			},
		}
		_, err := testTask.DockerConfig(testTask.Containers[0])
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
		Containers: []*Container{
			&Container{
				Name:        "c1",
				Environment: make(map[string]string),
			},
			&Container{
				Name:        "c2",
				Environment: make(map[string]string),
			}},
		credentialsId: credentialsIDInTask,
	}

	taskCredentials := &credentials.TaskIAMRoleCredentials{
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
		Containers: []*Container{
			&Container{
				Name:        "c1",
				Environment: make(map[string]string),
			},
			&Container{
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

// TODO: UT for PostUnmarshalTask, etc

func TestPostUnmarshalTaskWithEmptyVolumes(t *testing.T) {
	// Constants used here are defined in task_unix_test.go and task_windows_test.go
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			&ecsacs.Container{
				Name: strptr("myName1"),
				MountPoints: []*ecsacs.MountPoint{
					&ecsacs.MountPoint{
						ContainerPath: strptr(emptyVolumeContainerPath1),
						SourceVolume:  strptr(emptyVolumeName1),
					},
				},
			},
			&ecsacs.Container{
				Name: strptr("myName2"),
				MountPoints: []*ecsacs.MountPoint{
					&ecsacs.MountPoint{
						ContainerPath: strptr(emptyVolumeContainerPath2),
						SourceVolume:  strptr(emptyVolumeName2),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			&ecsacs.Volume{
				Name: strptr(emptyVolumeName1),
				Host: &ecsacs.HostVolumeProperties{},
			},
			&ecsacs.Volume{
				Name: strptr(emptyVolumeName2),
				Host: &ecsacs.HostVolumeProperties{},
			},
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 2, len(task.Containers)) // before PostUnmarshalTask
	task.PostUnmarshalTask(nil)

	assert.Equal(t, 3, len(task.Containers), "Should include new container for volumes")
	emptyContainer, ok := task.ContainerByName(emptyHostVolumeName)
	assert.True(t, ok, "Should find empty volume container")
	assert.Equal(t, 2, len(emptyContainer.MountPoints), "Should have two mount points")
	assert.Equal(t, []MountPoint{
		{
			SourceVolume:  emptyVolumeName1,
			ContainerPath: expectedEmptyVolumeGeneratedPath1,
		}, {
			SourceVolume:  emptyVolumeName2,
			ContainerPath: expectedEmptyVolumeGeneratedPath2,
		},
	}, emptyContainer.MountPoints)

}

func TestTaskFromACS(t *testing.T) {
	testTime := ttime.Now().Truncate(1 * time.Second).Format(time.RFC3339)

	intptr := func(i int64) *int64 {
		return &i
	}
	boolptr := func(b bool) *bool {
		return &b
	}
	// Testing type conversions, bleh. At least the type conversion itself
	// doesn't look this messy.
	taskFromAcs := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			&ecsacs.Container{
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
					&ecsacs.MountPoint{
						ContainerPath: strptr("/container/path"),
						ReadOnly:      boolptr(true),
						SourceVolume:  strptr("sourceVolume"),
					},
				},
				Overrides: strptr(`{"command":["a","b","c"]}`),
				PortMappings: []*ecsacs.PortMapping{
					&ecsacs.PortMapping{
						HostPort:      intptr(800),
						ContainerPort: intptr(900),
						Protocol:      strptr("udp"),
					},
				},
				VolumesFrom: []*ecsacs.VolumeFrom{
					&ecsacs.VolumeFrom{
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
			&ecsacs.Volume{
				Name: strptr("volName"),
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
	}
	expectedTask := &Task{
		Arn:           "myArn",
		DesiredStatus: TaskRunning,
		Family:        "myFamily",
		Version:       "1",
		Containers: []*Container{
			&Container{
				Name:        "myName",
				Image:       "image:tag",
				Command:     []string{"command", "command2"},
				Links:       []string{"link1", "link2"},
				EntryPoint:  &[]string{"sh", "-c"},
				Essential:   true,
				Environment: map[string]string{"key": "value"},
				Cpu:         10,
				Memory:      100,
				MountPoints: []MountPoint{
					MountPoint{
						ContainerPath: "/container/path",
						ReadOnly:      true,
						SourceVolume:  "sourceVolume",
					},
				},
				Overrides: ContainerOverrides{
					Command: &[]string{"a", "b", "c"},
				},
				Ports: []PortBinding{
					PortBinding{
						HostPort:      800,
						ContainerPort: 900,
						Protocol:      TransportProtocolUDP,
					},
				},
				VolumesFrom: []VolumeFrom{
					VolumeFrom{
						ReadOnly:        true,
						SourceContainer: "volumeLink",
					},
				},
				DockerConfig: DockerConfig{
					Config:     strptr("config json"),
					HostConfig: strptr("hostconfig json"),
					Version:    strptr("version string"),
				},
			},
		},
		Volumes: []TaskVolume{
			TaskVolume{
				Name: "volName",
				Volume: &FSHostVolume{
					FSSourcePath: "/host/path",
				},
			},
		},
		StartSequenceNumber: 42,
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromAcs, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	if err != nil {
		t.Fatalf("Should be able to handle acs task: %v", err)
	}
	if !reflect.DeepEqual(task.Containers, expectedTask.Containers) {
		t.Fatal("Containers should be equal")
	}
	if !reflect.DeepEqual(task.Volumes, expectedTask.Volumes) {
		t.Fatal("Volumes should be equal")
	}
	if !reflect.DeepEqual(task.StartSequenceNumber, expectedTask.StartSequenceNumber) {
		t.Fatal("StartSequenceNumber should be equal")
	}
	if !reflect.DeepEqual(task.StopSequenceNumber, expectedTask.StopSequenceNumber) {
		t.Fatal("StopSequenceNumber should be equal")
	}
}

func TestTaskUpdateKnownStatusHappyPath(t *testing.T) {
	testTask := &Task{
		KnownStatus: TaskStatusNone,
		Containers: []*Container{
			&Container{
				KnownStatus: ContainerCreated,
			},
			&Container{
				KnownStatus: ContainerStopped,
				Essential:   true,
			},
			&Container{
				KnownStatus: ContainerRunning,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, TaskCreated, newStatus, "task status should depend on the earlist container status")
	assert.Equal(t, TaskCreated, testTask.GetKnownStatus(), "task status should depend on the earlist container status")
}

// TestTaskUpdateKnownStatusNotChangeToRunningWithEssentialContainerStopped tests when there is one essential
// container is stopped while the other containers are running, the task status shouldn't be changed to running
func TestTaskUpdateKnownStatusNotChangeToRunningWithEssentialContainerStopped(t *testing.T) {
	testTask := &Task{
		KnownStatus: TaskCreated,
		Containers: []*Container{
			&Container{
				KnownStatus: ContainerRunning,
				Essential:   true,
			},
			&Container{
				KnownStatus: ContainerStopped,
				Essential:   true,
			},
			&Container{
				KnownStatus: ContainerRunning,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, TaskStatusNone, newStatus, "task status should not move to running if essential container is stopped")
	assert.Equal(t, TaskCreated, testTask.GetKnownStatus(), "task status should not move to running if essential container is stopped")
}

// TestTaskUpdateKnownStatusToPendingWithEssentialContainerStopped tests when there is one essential container
// is stopped while other container status are prior to Running, the task status should be updated.
func TestTaskUpdateKnownStatusToPendingWithEssentialContainerStopped(t *testing.T) {
	testTask := &Task{
		KnownStatus: TaskStatusNone,
		Containers: []*Container{
			&Container{
				KnownStatus: ContainerCreated,
				Essential:   true,
			},
			&Container{
				KnownStatus: ContainerStopped,
				Essential:   true,
			},
			&Container{
				KnownStatus: ContainerCreated,
			},
		},
	}

	newStatus := testTask.updateTaskKnownStatus()
	assert.Equal(t, TaskCreated, newStatus, "task status should be updated when essential containers are stopped while not all the other containers are running")
	assert.Equal(t, TaskCreated, testTask.GetKnownStatus(), "task status should be updated when essential containers are stopped while not all the other containers are running")
}

func assertSetStructFieldsEqual(t *testing.T, expected, actual interface{}) {
	for i := 0; i < reflect.TypeOf(expected).NumField(); i++ {
		expectedValue := reflect.ValueOf(expected).Field(i)
		// All the values we actaully expect to see are valid and non-nil
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
