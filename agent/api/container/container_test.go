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

package container

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/container/restart"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/docker/docker/api/types"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configPair struct {
	Container  *Container
	Config     *dockercontainer.Config
	HostConfig *dockercontainer.HostConfig
}

func (pair configPair) Equal() bool {
	conf := pair.Config
	cont := pair.Container
	hostConf := pair.HostConfig

	if (hostConf.Memory / 1024 / 1024) != int64(cont.Memory) {
		return false
	}
	if hostConf.CPUShares != int64(cont.CPU) {
		return false
	}
	if conf.Image != cont.Image {
		return false
	}
	if cont.EntryPoint == nil && !utils.StrSliceEqual(conf.Entrypoint, []string{}) {
		return false
	}
	if cont.EntryPoint != nil && !utils.StrSliceEqual(conf.Entrypoint, *cont.EntryPoint) {
		return false
	}
	if !utils.StrSliceEqual(cont.Command, conf.Cmd) {
		return false
	}
	// TODO, Volumes, VolumesFrom, ExposedPorts

	return true
}

func TestGetSteadyStateStatusReturnsRunningByDefault(t *testing.T) {
	container := &Container{}
	assert.Equal(t, container.GetSteadyStateStatus(), apicontainerstatus.ContainerRunning)
}

func TestIsKnownSteadyState(t *testing.T) {
	// This creates a container with `iota` ContainerStatus (NONE)
	container := &Container{}
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to PULLED, still not in steady state
	container.SetKnownStatus(apicontainerstatus.ContainerPulled)
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to CREATED, still not in steady state
	container.SetKnownStatus(apicontainerstatus.ContainerCreated)
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to RUNNING, now we're in steady state
	container.SetKnownStatus(apicontainerstatus.ContainerRunning)
	assert.True(t, container.IsKnownSteadyState())
	// Now, set steady state to RESOURCES_PROVISIONED
	resourcesProvisioned := apicontainerstatus.ContainerResourcesProvisioned
	container.SteadyStateStatusUnsafe = &resourcesProvisioned
	// Container is not in steady state anymore
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to RESOURCES_PROVISIONED, we're in
	// steady state again
	container.SetKnownStatus(apicontainerstatus.ContainerResourcesProvisioned)
	assert.True(t, container.IsKnownSteadyState())
}

func TestGetNextStateProgression(t *testing.T) {
	// This creates a container with `iota` ContainerStatus (NONE)
	container := &Container{}
	// NONE should transition to MANIFEST_PULLED
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerManifestPulled)
	container.SetKnownStatus(apicontainerstatus.ContainerManifestPulled)
	// MANIFEST_PULLED should transition to PULLED
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerPulled)
	container.SetKnownStatus(apicontainerstatus.ContainerPulled)
	// PULLED should transition to CREATED
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerCreated)
	container.SetKnownStatus(apicontainerstatus.ContainerCreated)
	// CREATED should transition to RUNNING
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerRunning)
	container.SetKnownStatus(apicontainerstatus.ContainerRunning)
	// RUNNING should transition to STOPPED
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerStopped)

	resourcesProvisioned := apicontainerstatus.ContainerResourcesProvisioned
	container.SteadyStateStatusUnsafe = &resourcesProvisioned
	// Set steady state to RESOURCES_PROVISIONED
	// RUNNING should transition to RESOURCES_PROVISIONED based on steady state
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerResourcesProvisioned)
	container.SetKnownStatus(apicontainerstatus.ContainerResourcesProvisioned)
	assert.Equal(t, container.GetNextKnownStateProgression(), apicontainerstatus.ContainerStopped)
}

func TestIsInternal(t *testing.T) {
	testCases := []struct {
		container *Container
		internal  bool
	}{
		{&Container{}, false},
		{&Container{Type: ContainerNormal}, false},
		{&Container{Type: ContainerCNIPause}, true},
		{&Container{Type: ContainerEmptyHostVolume}, true},
		{&Container{Type: ContainerNamespacePause}, true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("IsInternal should return %t for %s", tc.internal, tc.container.String()),
			func(t *testing.T) {
				assert.Equal(t, tc.internal, tc.container.IsInternal())
			})
	}
}

func TestIsManagedDaemonContainer(t *testing.T) {
	testCases := []struct {
		container       *Container
		internal        bool
		isManagedDaemon bool
	}{
		{&Container{}, false, false},
		{&Container{Type: ContainerNormal, Image: "someImage:latest"}, false, false},
		{&Container{Type: ContainerManagedDaemon, Image: "someImage:latest"}, true, true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("IsManagedDaemonContainer should return %t for %s", tc.isManagedDaemon, tc.container.String()),
			func(t *testing.T) {
				assert.Equal(t, tc.internal, tc.container.IsInternal())
				ok := tc.container.IsManagedDaemonContainer()
				assert.Equal(t, tc.isManagedDaemon, ok)
			})
	}
}

func TestGetImageName(t *testing.T) {
	testCases := []struct {
		container *Container
		imageName string
	}{
		{&Container{}, ""},
		{&Container{Image: "someImage:latest"}, "someImage"},
		{&Container{Image: "someImage"}, "someImage"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("GetImageName should return %s for %s", tc.imageName, tc.container.String()),
			func(t *testing.T) {
				imageName := tc.container.GetImageName()
				assert.Equal(t, tc.imageName, imageName)
			})
	}
}

// TestSetupExecutionRoleFlag tests whether or not the container appropriately
// sets the flag for using execution roles
func TestSetupExecutionRoleFlag(t *testing.T) {
	testCases := []struct {
		container *Container
		result    bool
		msg       string
	}{
		{&Container{}, false, "the container does not use ECR, so it should not require credentials"},
		{
			&Container{
				RegistryAuthentication: &RegistryAuthenticationData{Type: "non-ecr"},
			},
			false,
			"the container does not use ECR, so it should not require credentials",
		},
		{
			&Container{
				RegistryAuthentication: &RegistryAuthenticationData{Type: "ecr"},
			},
			false, "the container uses ECR, but it does not require execution role credentials",
		},
		{
			&Container{
				RegistryAuthentication: &RegistryAuthenticationData{
					Type: "ecr",
					ECRAuthData: &ECRAuthData{
						UseExecutionRole: true,
					},
				},
			},
			true,
			"the container uses ECR and require execution role credentials",
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("Container: %s", testCase.container.String()), func(t *testing.T) {
			assert.Equal(t, testCase.result, testCase.container.ShouldPullWithExecutionRole(), testCase.msg)
		})
	}
}

func TestSetHealthStatus(t *testing.T) {
	container := Container{}

	// set the container status to be healthy
	container.SetHealthStatus(HealthStatus{Status: apicontainerstatus.ContainerHealthy, Output: "test"})
	health := container.GetHealthStatus()
	assert.Equal(t, health.Status, apicontainerstatus.ContainerHealthy)
	assert.Equal(t, health.Output, "test")
	assert.NotEmpty(t, health.Since)

	// set the health status again shouldn't update the timestamp
	container.SetHealthStatus(HealthStatus{Status: apicontainerstatus.ContainerHealthy})
	health2 := container.GetHealthStatus()
	assert.Equal(t, health2.Status, apicontainerstatus.ContainerHealthy)
	assert.Equal(t, health2.Since, health.Since)

	// the sleep is to ensure the different of the two timestamp returned by time.Now()
	// is big enough to pass asser.NotEqual
	time.Sleep(10 * time.Millisecond)
	// change the container health status
	container.SetHealthStatus(HealthStatus{Status: apicontainerstatus.ContainerUnhealthy, ExitCode: 1})
	health3 := container.GetHealthStatus()
	assert.Equal(t, health3.Status, apicontainerstatus.ContainerUnhealthy)
	assert.Equal(t, health3.ExitCode, 1)
	assert.NotEqual(t, health3.Since, health2.Since)
}

func TestHealthStatusShouldBeReported(t *testing.T) {
	container := Container{}
	assert.False(t, container.HealthStatusShouldBeReported(), "Health status of container that does not have HealthCheckType set should not be reported")
	container.HealthCheckType = DockerHealthCheckType
	assert.True(t, container.HealthStatusShouldBeReported(), "Health status of container that has docker HealthCheckType set should be reported")
	container.HealthCheckType = "unknown"
	assert.False(t, container.HealthStatusShouldBeReported(), "Health status of container that has non-docker HealthCheckType set should not be reported")
}

func TestBuildContainerDependency(t *testing.T) {
	container := Container{TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]TransitionDependencySet)}
	depContName := "dep"
	container.BuildContainerDependency(depContName, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	assert.NotNil(t, container.TransitionDependenciesMap)
	contDep := container.TransitionDependenciesMap[apicontainerstatus.ContainerRunning].ContainerDependencies
	assert.Len(t, container.TransitionDependenciesMap, 1)
	assert.Len(t, contDep, 1)
	assert.Equal(t, contDep[0].ContainerName, depContName)
	assert.Equal(t, contDep[0].SatisfiedStatus, apicontainerstatus.ContainerRunning)
}

func TestBuildResourceDependency(t *testing.T) {
	container := Container{TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]TransitionDependencySet)}
	depResourceName := "cgroup"

	container.BuildResourceDependency(depResourceName, resourcestatus.ResourceStatus(1), apicontainerstatus.ContainerRunning)

	assert.NotNil(t, container.TransitionDependenciesMap)
	resourceDep := container.TransitionDependenciesMap[apicontainerstatus.ContainerRunning].ResourceDependencies
	assert.Len(t, container.TransitionDependenciesMap, 1)
	assert.Len(t, resourceDep, 1)
	assert.Equal(t, depResourceName, resourceDep[0].Name)
	assert.Equal(t, resourcestatus.ResourceStatus(1), resourceDep[0].GetRequiredStatus())
}

func TestShouldPullWithASMAuth(t *testing.T) {
	container := Container{
		Name:  "myName",
		Image: "image:tag",
		RegistryAuthentication: &RegistryAuthenticationData{
			Type: "asm",
			ASMAuthData: &ASMAuthData{
				CredentialsParameter: "secret-id",
				Region:               "region",
			},
		},
	}

	assert.True(t, container.ShouldPullWithASMAuth())
}

func TestInjectV3MetadataEndpoint(t *testing.T) {
	container := Container{
		V3EndpointID: "myV3EndpointID",
	}

	container.InjectV3MetadataEndpoint()

	assert.NotNil(t, container.Environment)
	assert.Equal(t, container.Environment[MetadataURIEnvironmentVariableName],
		fmt.Sprintf(MetadataURIFormat, "myV3EndpointID"))
}

func TestInjectV4MetadataEndpoint(t *testing.T) {
	container := Container{
		V3EndpointID: "EndpointID",
	}
	container.InjectV4MetadataEndpoint()

	assert.NotNil(t, container.Environment)
	assert.Equal(t, container.Environment[MetadataURIEnvVarNameV4],
		fmt.Sprintf(MetadataURIFormatV4, "EndpointID"))
}

func TestShouldCreateWithSSMSecret(t *testing.T) {
	cases := []struct {
		in  Container
		out bool
	}{
		{Container{
			Name:  "myName",
			Image: "image:tag",
			Secrets: []Secret{
				Secret{
					Provider:  "ssm",
					Name:      "secret",
					ValueFrom: "/test/secretName",
				}},
		}, true},
		{Container{
			Name:    "myName",
			Image:   "image:tag",
			Secrets: nil,
		}, false},
		{Container{
			Name:  "myName",
			Image: "image:tag",
			Secrets: []Secret{
				Secret{
					Provider:  "asm",
					Name:      "secret",
					ValueFrom: "/test/secretName",
				}},
		}, false},
	}

	for _, test := range cases {
		container := test.in
		assert.Equal(t, test.out, container.ShouldCreateWithSSMSecret())
	}
}

func TestMergeEnvironmentVariables(t *testing.T) {
	cases := []struct {
		Name                   string
		InContainerEnvironment map[string]string
		InEnvVarMap            map[string]string
		OutEnvVarMap           map[string]string
	}{
		{
			Name: "merge single item",
			InContainerEnvironment: map[string]string{
				"CONFIG1": "config1"},
			InEnvVarMap: map[string]string{
				"SECRET1": "secret1"},
			OutEnvVarMap: map[string]string{
				"CONFIG1": "config1",
				"SECRET1": "secret1",
			},
		},

		{
			Name:                   "merge single item to nil container env var map",
			InContainerEnvironment: nil,
			InEnvVarMap: map[string]string{
				"SECRET1": "secret1"},
			OutEnvVarMap: map[string]string{
				"SECRET1": "secret1",
			},
		},

		{
			Name: "merge zero items to existing container env var map",
			InContainerEnvironment: map[string]string{
				"CONFIG1": "config1"},
			InEnvVarMap: map[string]string{},
			OutEnvVarMap: map[string]string{
				"CONFIG1": "config1",
			},
		},

		{
			Name: "merge nil to existing container env var map",
			InContainerEnvironment: map[string]string{
				"CONFIG1": "config1"},
			InEnvVarMap: nil,
			OutEnvVarMap: map[string]string{
				"CONFIG1": "config1",
			},
		},

		{
			Name:                   "merge nil to nil container env var map",
			InContainerEnvironment: nil,
			InEnvVarMap:            nil,
			OutEnvVarMap:           map[string]string{},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			container := Container{
				Environment: c.InContainerEnvironment,
			}

			container.MergeEnvironmentVariables(c.InEnvVarMap)
			mapEq := reflect.DeepEqual(c.OutEnvVarMap, container.Environment)
			assert.True(t, mapEq)
		})
	}
}

func TestShouldCreateWithASMSecret(t *testing.T) {
	cases := []struct {
		in  Container
		out bool
	}{
		{Container{
			Name:  "myName",
			Image: "image:tag",
			Secrets: []Secret{
				Secret{
					Provider:  "asm",
					Name:      "secret",
					ValueFrom: "/test/secretName",
				}},
		}, true},
		{Container{
			Name:    "myName",
			Image:   "image:tag",
			Secrets: nil,
		}, false},
		{Container{
			Name:  "myName",
			Image: "image:tag",
			Secrets: []Secret{
				Secret{
					Provider:  "ssm",
					Name:      "secret",
					ValueFrom: "/test/secretName",
				}},
		}, false},
	}

	for _, test := range cases {
		container := test.in
		assert.Equal(t, test.out, container.ShouldCreateWithASMSecret())
	}
}

func TestHasSecret(t *testing.T) {
	isEnvOrLogDriverSecret := func(s Secret) bool {
		return s.Type == SecretTypeEnv || s.Target == SecretTargetLogDriver
	}
	isSSMLogDriverSecret := func(s Secret) bool {
		return s.Provider == SecretProviderSSM && s.Target == SecretTargetLogDriver
	}

	testCases := []struct {
		name    string
		f       func(s Secret) bool
		secrets []Secret
		res     bool
	}{
		{
			name: "test env secret",
			f:    isEnvOrLogDriverSecret,
			secrets: []Secret{
				{
					Provider:  "asm",
					Name:      "secret",
					Type:      "ENVIRONMENT_VARIABLE",
					ValueFrom: "/test/secretName",
				}},
			res: true,
		},
		{
			name: "test no secret",
			f:    isEnvOrLogDriverSecret,
		},
		{
			name: "test mount point secret",
			f:    isEnvOrLogDriverSecret,
			secrets: []Secret{
				{
					Provider:  "asm",
					Name:      "secret",
					Type:      "MOUNT_POINT",
					ValueFrom: "/test/secretName",
				}},
			res: false,
		},
		{
			name: "test log driver secret",
			f:    isEnvOrLogDriverSecret,
			secrets: []Secret{
				{
					Provider:  "asm",
					Name:      "splunk-token",
					ValueFrom: "/test/secretName",
					Target:    "LOG_DRIVER",
				}},
			res: true,
		},
		{
			name: "test secret provider ssm",
			f:    isSSMLogDriverSecret,
			secrets: []Secret{
				{
					Name:     "secret",
					Provider: SecretProviderSSM,
					Target:   SecretTargetLogDriver,
				},
			},
			res: true,
		},
		{
			name: "test wrong secret provider",
			f:    isSSMLogDriverSecret,
			secrets: []Secret{
				{
					Name:     "secret",
					Provider: "dummy",
					Target:   SecretTargetLogDriver,
				},
			},
			res: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Container{
				Name:    "c",
				Secrets: tc.secrets,
			}

			assert.Equal(t, tc.res, c.HasSecret(tc.f))
		})
	}
}

func TestPerContainerTimeouts(t *testing.T) {
	timeout := uint(10)
	expectedTimeout := time.Duration(timeout) * time.Second

	container := Container{
		StartTimeout: timeout,
		StopTimeout:  timeout,
	}

	assert.Equal(t, container.GetStartTimeout(), expectedTimeout)
	assert.Equal(t, container.GetStopTimeout(), expectedTimeout)
}

func TestSetRuntimeIDInContainer(t *testing.T) {
	container := Container{}
	container.SetRuntimeID("asdfghjkl1234")
	assert.Equal(t, "asdfghjkl1234", container.RuntimeID)
	assert.Equal(t, "asdfghjkl1234", container.GetRuntimeID())
}

func TestGetManagedAgents(t *testing.T) {
	container := Container{}
	assert.Nil(t, container.GetManagedAgents())

	expectedManagedAgent := ManagedAgent{
		Name:       "dummyAgent",
		Properties: map[string]string{"test": "prop"},
		ManagedAgentState: ManagedAgentState{
			LastStartedAt: time.Now(),
			Status:        apicontainerstatus.ManagedAgentCreated,
		},
	}
	container.ManagedAgentsUnsafe = []ManagedAgent{expectedManagedAgent}
	assert.Equal(t, expectedManagedAgent, container.GetManagedAgents()[0])
}

func TestGetManagedAgentStatus(t *testing.T) {
	container := Container{}
	assert.Equal(t, apicontainerstatus.ManagedAgentStatusNone, container.GetManagedAgentStatus("dummyAgent"))

	expectedManagedAgent := ManagedAgent{
		Name: "dummyAgent",
		ManagedAgentState: ManagedAgentState{
			Status: apicontainerstatus.ManagedAgentCreated,
		},
	}
	container.ManagedAgentsUnsafe = []ManagedAgent{expectedManagedAgent}
	assert.Equal(t, apicontainerstatus.ManagedAgentCreated, container.GetManagedAgentStatus("dummyAgent"))
}

func TestGetManagedAgentSentStatus(t *testing.T) {
	container := Container{}
	assert.Equal(t, apicontainerstatus.ManagedAgentStatusNone, container.GetManagedAgentSentStatus("dummyAgent"))

	expectedManagedAgent := ManagedAgent{
		Name: "dummyAgent",
		ManagedAgentState: ManagedAgentState{
			SentStatus: apicontainerstatus.ManagedAgentCreated,
		},
	}
	container.ManagedAgentsUnsafe = []ManagedAgent{expectedManagedAgent}
	assert.Equal(t, apicontainerstatus.ManagedAgentCreated, container.GetManagedAgentSentStatus("dummyAgent"))
}

func TestDependsOnContainer(t *testing.T) {
	testCases := []struct {
		name          string
		container     *Container
		dependsOnName string
		dependsOn     bool
	}{
		{
			name: "test DependsOnContainer positive case",
			container: &Container{
				Name: "container1",
				DependsOnUnsafe: []DependsOn{
					{
						ContainerName: "container2",
						Condition:     "START",
					},
				},
			},
			dependsOnName: "container2",
			dependsOn:     true,
		},
		{
			name: "test DependsOnContainer negative case",
			container: &Container{
				Name: "container1",
				DependsOnUnsafe: []DependsOn{
					{
						ContainerName: "container2",
						Condition:     "START",
					},
				},
			},
			dependsOnName: "container0",
			dependsOn:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.dependsOn, tc.container.DependsOnContainer(tc.dependsOnName))
		})
	}
}

func TestAddContainerDependency(t *testing.T) {
	container := &Container{
		Name: "container1",
	}
	container.AddContainerDependency("container2", "START")

	assert.Contains(t, container.DependsOnUnsafe, DependsOn{
		ContainerName: "container2",
		Condition:     "START",
	})
}

func TestGetLogDriver(t *testing.T) {
	getContainer := func(hostConfig string) *Container {
		c := &Container{
			Name: "c",
		}
		c.DockerConfig.HostConfig = &hostConfig
		return c
	}

	testCases := []struct {
		name      string
		container *Container
		logDriver string
	}{
		{
			name:      "positive case",
			container: getContainer(`{"LogConfig":{"Type":"logdriver"}}`),
			logDriver: "logdriver",
		},
		{
			name:      "negative case",
			container: getContainer("invalid"),
			logDriver: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.logDriver, tc.container.GetLogDriver())
		})
	}
}

func TestGetNetworkModeFromHostConfig(t *testing.T) {
	getContainer := func(hostConfig string) *Container {
		c := &Container{
			Name: "c",
		}
		c.DockerConfig.HostConfig = &hostConfig
		return c
	}

	testCases := []struct {
		name           string
		container      *Container
		expectedOutput string
	}{
		{
			name:           "bridge mode",
			container:      getContainer("{\"NetworkMode\":\"bridge\"}"),
			expectedOutput: "bridge",
		},
		{
			name:           "invalid case",
			container:      getContainer("invalid"),
			expectedOutput: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.container.GetNetworkModeFromHostConfig())
		})
	}
}

func TestGetMemoryReservationFromHostConfig(t *testing.T) {
	getContainer := func(hostConfig *string) *Container {
		c := &Container{
			Name: "c",
		}
		c.DockerConfig.HostConfig = hostConfig
		return c
	}

	getStrPtr := func(s string) *string {
		return &s
	}

	testCases := []struct {
		name           string
		container      *Container
		expectedOutput int64
	}{
		{
			name:           "happy case",
			container:      getContainer(getStrPtr("{\"MemoryReservation\":268435456}")),
			expectedOutput: 256,
		},
		{
			name:           "invalid case",
			container:      getContainer(getStrPtr("invalid")),
			expectedOutput: 0,
		},
		{
			name:           "nil case",
			container:      getContainer(nil),
			expectedOutput: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.container.GetMemoryReservationFromHostConfig())
		})
	}
}

func TestShouldCreateWithEnvfiles(t *testing.T) {
	cases := []struct {
		in  Container
		out bool
	}{
		{
			Container{
				Name:  "containerName",
				Image: "image:tag",
				EnvironmentFiles: []EnvironmentFile{
					EnvironmentFile{
						Value: "s3://bucket/envfile",
						Type:  "s3",
					},
				},
			}, true},
		{
			Container{
				Name:             "containerName",
				Image:            "image:tag",
				EnvironmentFiles: nil,
			}, false},
	}

	for _, test := range cases {
		container := test.in
		assert.Equal(t, test.out, container.ShouldCreateWithEnvFiles())
	}
}

func TestMergeEnvironmentVariablesFromEnvfiles(t *testing.T) {
	cases := []struct {
		Name                   string
		InContainerEnvironment map[string]string
		InEnvVarList           []map[string]string
		OutEnvVarMap           map[string]string
	}{
		{
			Name:                   "merge one item",
			InContainerEnvironment: map[string]string{"key1": "value1"},
			InEnvVarList:           []map[string]string{{"key2": "value2"}},
			OutEnvVarMap: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			Name:                   "merge single item to nil env var map",
			InContainerEnvironment: nil,
			InEnvVarList:           []map[string]string{{"key": "value"}},
			OutEnvVarMap:           map[string]string{"key": "value"},
		},
		{
			Name:                   "merge one item key already exists",
			InContainerEnvironment: map[string]string{"key1": "value1"},
			InEnvVarList:           []map[string]string{{"key1": "value2"}},
			OutEnvVarMap:           map[string]string{"key1": "value1"},
		},
		{
			Name:                   "merge two items with same key",
			InContainerEnvironment: map[string]string{"key1": "value1"},
			InEnvVarList: []map[string]string{
				{"key2": "value2"},
				{"key2": "value3"},
			},
			OutEnvVarMap: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			container := Container{
				Environment: test.InContainerEnvironment,
			}

			container.MergeEnvironmentVariablesFromEnvfiles(test.InEnvVarList)
			assert.True(t, reflect.DeepEqual(test.OutEnvVarMap, container.Environment))
		})
	}
}

func TestRequireNeuronRuntime(t *testing.T) {
	c := &Container{
		Environment: map[string]string{neuronVisibleDevicesEnvVar: "all"},
	}
	assert.True(t, c.RequireNeuronRuntime())
}

func TestHasNotAndWillNotStart(t *testing.T) {
	testCases := []struct {
		name          string
		knownStatus   apicontainerstatus.ContainerStatus
		desiredStatus apicontainerstatus.ContainerStatus
		appliedStatus apicontainerstatus.ContainerStatus
		expected      bool
	}{
		{
			name:          "container has started",
			knownStatus:   apicontainerstatus.ContainerRunning,
			desiredStatus: apicontainerstatus.ContainerRunning,
			appliedStatus: apicontainerstatus.ContainerStatusNone,
		},
		{
			name:          "container wants to start",
			knownStatus:   apicontainerstatus.ContainerCreated,
			desiredStatus: apicontainerstatus.ContainerRunning,
			appliedStatus: apicontainerstatus.ContainerStatusNone,
		},
		{
			name:          "container in the middle of transition",
			knownStatus:   apicontainerstatus.ContainerCreated,
			desiredStatus: apicontainerstatus.ContainerStopped,
			appliedStatus: apicontainerstatus.ContainerRunning,
		},
		{
			name:          "container has not and will not start",
			knownStatus:   apicontainerstatus.ContainerPulled,
			desiredStatus: apicontainerstatus.ContainerStopped,
			appliedStatus: apicontainerstatus.ContainerStatusNone,
			expected:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cont := &Container{
				KnownStatusUnsafe:   tc.knownStatus,
				DesiredStatusUnsafe: tc.desiredStatus,
				AppliedStatus:       tc.appliedStatus,
			}
			assert.Equal(t, tc.expected, cont.HasNotAndWillNotStart())
		})
	}
}

func TestUpdateManagedAgentByName(t *testing.T) {
	const (
		dummyAgent = "dummyAgent"
		testStatus = apicontainerstatus.ManagedAgentStopped
		testReason = "reason"
	)
	cases := []struct {
		name      string
		agentName string
		state     ManagedAgentState
	}{
		{
			name:      "test nonexistent managed agent",
			agentName: "nonexistentAgent",
		},
		{
			name:      "test managed agent with default (zero) state",
			agentName: dummyAgent,
		},
		{
			name:      "test managed agent with nil metadata",
			agentName: dummyAgent,
			state: ManagedAgentState{
				Status:        testStatus,
				Reason:        testReason,
				LastStartedAt: time.Time{},
				Metadata:      nil,
			},
		},
		{
			name:      "test managed agent with full state",
			agentName: dummyAgent,
			state: ManagedAgentState{
				Status:        testStatus,
				Reason:        testReason,
				LastStartedAt: time.Time{},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := &Container{}

			// Verify we don't have extraneous data on a blank Container
			assert.Nil(t, c.GetManagedAgents())
			agent, ok := c.GetManagedAgentByName(test.agentName)
			assert.False(t, ok)
			assert.Equal(t, ManagedAgentState{}, agent.ManagedAgentState)
			var expectAgentFound bool
			// simulate we only have data for "dummyAgent"
			if test.agentName == dummyAgent {
				expectAgentFound = true
				c.ManagedAgentsUnsafe = []ManagedAgent{
					{
						Name:              test.agentName,
						ManagedAgentState: test.state,
					},
				}
			}

			// Verify that we can retrieve the correct data stored in container.ManagedAgentsUnsafe
			agent, ok = c.GetManagedAgentByName(test.agentName)
			assert.Equal(t, expectAgentFound, ok)
			assert.Equal(t, test.state, agent.ManagedAgentState)

			var newState ManagedAgentState
			// simulate we only replace data data for "dummyAgent"
			if test.agentName == dummyAgent {
				newState = ManagedAgentState{
					Status:        apicontainerstatus.ManagedAgentRunning,
					Reason:        "new reason",
					LastStartedAt: time.Now(),
				}
				c.UpdateManagedAgentByName(test.agentName, newState)
			}
			agent, ok = c.GetManagedAgentByName(test.agentName)
			assert.Equal(t, expectAgentFound, ok)
			assert.Equal(t, newState, agent.ManagedAgentState)
		})
	}
}

func TestUpdateManagedAgentSentStatus(t *testing.T) {
	const dummyAgent = "dummyAgent"
	cases := []struct {
		name               string
		agentName          string
		updateSentStatus   bool
		sentStatus         apicontainerstatus.ManagedAgentStatus
		expectedSentStatus apicontainerstatus.ManagedAgentStatus
	}{
		{
			name:               "test nonexistent managed agent",
			agentName:          "nonexistentAgent",
			expectedSentStatus: apicontainerstatus.ManagedAgentStatusNone,
		},
		{
			name:               "test managed agent with default (zero) status",
			agentName:          dummyAgent,
			expectedSentStatus: apicontainerstatus.ManagedAgentStatusNone,
		},
		{
			name:               "test managed agent with custom status",
			agentName:          dummyAgent,
			updateSentStatus:   true,
			sentStatus:         apicontainerstatus.ManagedAgentRunning,
			expectedSentStatus: apicontainerstatus.ManagedAgentRunning,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Container{
				ManagedAgentsUnsafe: []ManagedAgent{{
					Name: tc.agentName,
				}},
			}
			before, _ := c.GetManagedAgentByName(tc.agentName)
			assert.Equal(t, apicontainerstatus.ManagedAgentStatusNone, before.SentStatus)
			if tc.updateSentStatus {
				c.UpdateManagedAgentSentStatus(tc.agentName, tc.sentStatus)
			}
			after, _ := c.GetManagedAgentByName(tc.agentName)
			assert.Equal(t, tc.expectedSentStatus, after.SentStatus)

		})
	}
}

func TestRequiresAnyCredentialSpec(t *testing.T) {
	testCases := []struct {
		name           string
		container      *Container
		expectedOutput bool
	}{
		{
			name:           "hostconfig_nil",
			container:      &Container{},
			expectedOutput: false,
		},
		{
			name:           "invalid_case",
			container:      getContainer("invalid", nil),
			expectedOutput: false,
		},
		{
			name:           "empty_sec_opt",
			container:      getContainer("{\"NetworkMode\":\"bridge\"}", nil),
			expectedOutput: false,
		},
		{
			name:           "missing_credentialspec",
			container:      getContainer("{\"SecurityOpt\": [\"invalid-sec-opt\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "wrong_prefix",
			container:      getContainer("{\"SecurityOpt\": [\"credential2spec:file://gmsa_gmsa-acct.json\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_file",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}", nil),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_s3",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}\"]}", nil),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_ssm",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/parameter_name\"]}", nil),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_file_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspec:file://gmsa-acct.json"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_file_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:file://gmsa-acct.json"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_s3_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_s3_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:s3:::${BucketName}/${ObjectName}"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_ssm_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_ssm_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name"}),
			expectedOutput: true,
		},
		{
			name:           "host_config_credentialspecs_file",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:file://hostconfig.json\"]}", []string{"credentialspecdomainless:file://credentialspecs.json"}),
			expectedOutput: true,
		},
		{
			name:           "host_config_credentialspecs_s3",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::hostconfig.json\"]}", []string{"credentialspec:arn:aws:s3:::credentialspecs.json"}),
			expectedOutput: true,
		},
		{
			name:           "host_config_credentialspecs_ssm",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/hostconfig.json\"]}", []string{"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/credentialspecs.json"}),
			expectedOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.container.RequiresAnyCredentialSpec())
		})
	}
}

func TestRequiresDomainlessCredentialSpec(t *testing.T) {
	testCases := []struct {
		name           string
		container      *Container
		expectedOutput bool
	}{
		{
			name:           "hostconfig_nil",
			container:      &Container{},
			expectedOutput: false,
		},
		{
			name:           "invalid_case",
			container:      getContainer("invalid", nil),
			expectedOutput: false,
		},
		{
			name:           "empty_sec_opt",
			container:      getContainer("{\"NetworkMode\":\"bridge\"}", nil),
			expectedOutput: false,
		},
		{
			name:           "missing_credentialspec",
			container:      getContainer("{\"SecurityOpt\": [\"invalid-sec-opt\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "wrong_prefix",
			container:      getContainer("{\"SecurityOpt\": [\"credential2spec:file://gmsa_gmsa-acct.json\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_file",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_s3",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_ssm",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/parameter_name\"]}", nil),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_file_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspec:file://gmsa-acct.json"}),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_file_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:file://gmsa-acct.json"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_s3_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}"}),
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec_s3_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:s3:::${BucketName}/${ObjectName}"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_ssm_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name"}),
			expectedOutput: true,
		},
		{
			name:           "valid_credentialspec_ssm_domainless_not_hostconfig",
			container:      getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name"}),
			expectedOutput: true,
		},
		{
			name:           "host_config_credentialspecs_file",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:file://hostconfig.json\"]}", []string{"credentialspecdomainless:file://credentialspecs.json"}),
			expectedOutput: true,
		},
		{
			name:           "host_config_credentialspecs_s3",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::hostconfig.json\"]}", []string{"credentialspec:arn:aws:s3:::credentialspecs.json"}),
			expectedOutput: false,
		},
		{
			name:           "host_config_credentialspecs_ssm",
			container:      getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/hostconfig.json\"]}", []string{"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/credentialspecs.json"}),
			expectedOutput: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.container.RequiresDomainlessCredentialSpec())
		})
	}
}

func TestGetCredentialSpecErr(t *testing.T) {
	testCases := []struct {
		name                 string
		container            *Container
		expectedOutputString string
		expectedErrorString  string
	}{
		{
			name:                 "hostconfig_nil",
			container:            &Container{},
			expectedOutputString: "",
			expectedErrorString:  "unable to obtain credentialspec from both hostConfig and credentialSpecs",
		},
		{
			name:                 "invalid_case",
			container:            getContainer("invalid", nil),
			expectedOutputString: "",
			expectedErrorString:  "unable to obtain credentialspec from both hostConfig and credentialSpecs",
		},
		{
			name:                 "empty_sec_opt",
			container:            getContainer("{\"NetworkMode\":\"bridge\"}", nil),
			expectedOutputString: "",
			expectedErrorString:  "unable to obtain credentialspec from both hostConfig and credentialSpecs",
		},
		{
			name:                 "missing_credentialspec",
			container:            getContainer("{\"SecurityOpt\": [\"invalid-sec-opt\"]}", nil),
			expectedOutputString: "",
			expectedErrorString:  "unable to obtain credentialspec from both hostConfig and credentialSpecs",
		},
		{
			name:                 "invalid_case_credential_specs",
			container:            getContainer("{\"NetworkMode\":\"bridge\"}", []string{"invalid"}),
			expectedOutputString: "",
			expectedErrorString:  "unable to obtain credentialspec from both hostConfig and credentialSpecs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedOutputStr, err := tc.container.GetCredentialSpec()
			assert.Equal(t, tc.expectedOutputString, expectedOutputStr)
			assert.EqualError(t, err, tc.expectedErrorString)
		})
	}
}

func TestGetCredentialSpec(t *testing.T) {
	testCases := []struct {
		name                 string
		container            *Container
		expectedOutputString string
	}{
		{
			name:                 "hostconfig_file",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}", nil),
			expectedOutputString: "credentialspec:file://gmsa_gmsa-acct.json",
		},
		{
			name:                 "hostconfig_file_multiple",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\", \"credentialspec:file://gmsa_gmsa-acct2.json\"]}", nil),
			expectedOutputString: "credentialspec:file://gmsa_gmsa-acct.json",
		},
		{
			name:                 "hostconfig_s3",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}\"]}", nil),
			expectedOutputString: "credentialspec:arn:aws:s3:::${BucketName}/${ObjectName}",
		},
		{
			name:                 "hostconfig_ssm",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/parameter_name\"]}", nil),
			expectedOutputString: "credentialspec:arn:aws:ssm:region:aws_account_id:parameter/parameter_name",
		},
		{
			name:                 "credentialspecs_file",
			container:            getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:file://gmsa_gmsa-acct.json"}),
			expectedOutputString: "credentialspecdomainless:file://gmsa_gmsa-acct.json",
		},
		{
			name:                 "credentialspecs_file_multiple",
			container:            getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:file://gmsa_gmsa-acct.json", "credentialspecdomainless:file://gmsa_gmsa-acct2.json"}),
			expectedOutputString: "credentialspecdomainless:file://gmsa_gmsa-acct.json",
		},
		{
			name:                 "credentialspecs_s3",
			container:            getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:s3:::${BucketName}/${ObjectName}"}),
			expectedOutputString: "credentialspecdomainless:arn:aws:s3:::${BucketName}/${ObjectName}",
		},
		{
			name:                 "credentialspecs_ssm",
			container:            getContainer("{\"SecurityOpt\": []}", []string{"credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name"}),
			expectedOutputString: "credentialspecdomainless:arn:aws:ssm:region:aws_account_id:parameter/parameter_name",
		},
		{
			name:                 "host_config_credentialspecs_file",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:file://hostconfig.json\"]}", []string{"credentialspecdomainless:file://credentialspecs.json"}),
			expectedOutputString: "credentialspecdomainless:file://credentialspecs.json",
		},
		{
			name:                 "host_config_credentialspecs_s3",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:s3:::hostconfig.json\"]}", []string{"credentialspec:arn:aws:s3:::credentialspecs.json"}),
			expectedOutputString: "credentialspec:arn:aws:s3:::credentialspecs.json",
		},
		{
			name:                 "host_config_credentialspecs_ssm",
			container:            getContainer("{\"SecurityOpt\": [\"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/hostconfig.json\"]}", []string{"credentialspec:arn:aws:ssm:region:aws_account_id:parameter/credentialspecs.json"}),
			expectedOutputString: "credentialspec:arn:aws:ssm:region:aws_account_id:parameter/credentialspecs.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedOutputStr, err := tc.container.GetCredentialSpec()
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOutputString, expectedOutputStr)
		})
	}
}

func TestRestartPolicy(t *testing.T) {
	testCases := []struct {
		name            string
		container       *Container
		restartCount    int
		expectedEnabled bool
	}{
		{
			name: "nil restart policy",
			container: &Container{
				RestartPolicy: nil,
			},
			restartCount:    0,
			expectedEnabled: false,
		},
		{
			name: "not enabled restart policy",
			container: &Container{
				RestartPolicy: &restart.RestartPolicy{},
			},
			restartCount:    0,
			expectedEnabled: false,
		},
		{
			name: "explicitly not enabled restart policy",
			container: &Container{
				RestartPolicy: &restart.RestartPolicy{
					Enabled: false,
				},
			},
			restartCount:    0,
			expectedEnabled: false,
		},
		{
			name: "enabled restart policy",
			container: &Container{
				RestartPolicy: &restart.RestartPolicy{
					Enabled: true,
				},
			},
			restartCount:    0,
			expectedEnabled: true,
		},
		{
			name: "enabled restart policy, record 5 restarts",
			container: &Container{
				RestartPolicy: &restart.RestartPolicy{
					Enabled: true,
				},
			},
			restartCount:    5,
			expectedEnabled: true,
		},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.expectedEnabled, tc.container.RestartPolicyEnabled())
		if tc.container.RestartPolicyEnabled() {
			tc.container.RestartTracker = restart.NewRestartTracker(*tc.container.RestartPolicy)
			for i := 0; i < tc.restartCount; i++ {
				tc.container.RestartTracker.RecordRestart()
			}
			require.Equal(t, tc.restartCount, tc.container.RestartTracker.GetRestartCount())
		}
	}
}

func TestGetAndSetStartedAt(t *testing.T) {
	testTime := time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)
	c := &Container{}

	// Test getting started at time when it has never been set is the zero value of time.
	require.True(t, c.GetStartedAt().IsZero())

	// Test setting started at time sets the started at time.
	c.SetStartedAt(testTime)
	require.Equal(t, testTime, c.GetStartedAt())

	// Test setting started at time after it has already been set does not change the originally set started at time.
	c.SetStartedAt(time.Now())
	require.Equal(t, testTime, c.GetStartedAt())
}

func TestGetAndSetRestartAggregationDataForStats(t *testing.T) {
	testTime := time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)
	testStatsJSON := types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage: 100,
				},
			},
			MemoryStats: types.MemoryStats{
				MaxUsage: 200,
			},
		},
	}
	testRestartAggregationDataForStats := ContainerRestartAggregationDataForStats{
		LastRestartDetectedAt:     testTime,
		LastStatBeforeLastRestart: testStatsJSON,
	}
	c := &Container{}

	// Test getting restart aggregation data for stats when it has never been set is the zero value of the
	// ContainerRestartAggregationDataForStats struct.
	require.Equal(t, ContainerRestartAggregationDataForStats{}, c.GetRestartAggregationDataForStats())

	// Test setting restart aggregation data for stats sets the restart aggregation data for stats.
	c.SetRestartAggregationDataForStats(testRestartAggregationDataForStats)
	require.Equal(t, testRestartAggregationDataForStats, c.GetRestartAggregationDataForStats())
}

func getContainer(hostConfig string, credentialSpecs []string) *Container {
	c := &Container{
		Name: "c",
	}
	c.DockerConfig.HostConfig = &hostConfig
	c.CredentialSpecs = credentialSpecs
	return c
}

func TestDigestResolved(t *testing.T) {
	t.Run("never resolved for internal container", func(t *testing.T) {
		assert.False(t, (&Container{Type: ContainerServiceConnectRelay}).DigestResolved())
	})
	t.Run("digest resolved if it is populated", func(t *testing.T) {
		assert.True(t, (&Container{ImageDigest: "digest"}).DigestResolved())
	})
	t.Run("digest not resolved if it is not populated", func(t *testing.T) {
		assert.False(t, (&Container{}).DigestResolved())
	})
	t.Run("never resolved for container if digest found in image reference", func(t *testing.T) {
		image := "ubuntu@sha256:ed6d2c43c8fbcd3eaa44c9dab6d94cb346234476230dc1681227aa72d07181ee"
		imageDigest := "sha256:ed6d2c43c8fbcd3eaa44c9dab6d94cb346234476230dc1681227aa72d07181ee"
		assert.False(t, (&Container{Image: image, ImageDigest: imageDigest}).DigestResolved())
	})
}

func TestGetUser(t *testing.T) {
	tests := []struct {
		name         string
		dockerConfig string
		expectedUser string
	}{
		{
			name: "Valid user configuration",
			dockerConfig: func() string {
				config := &dockercontainer.Config{
					User: "12345",
				}
				configBytes, _ := json.Marshal(config)
				return string(configBytes)
			}(),
			expectedUser: "12345",
		},
		{
			name:         "Empty config",
			dockerConfig: "",
			expectedUser: "",
		},
		{
			name:         "Invalid JSON config",
			dockerConfig: "invalid-json",
			expectedUser: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := &Container{
				Name: "foo",
			}

			if tc.dockerConfig != "" {
				container.DockerConfig.Config = &tc.dockerConfig
			}

			result := container.GetUser()
			assert.Equal(t, tc.expectedUser, result)
		})
	}
}
