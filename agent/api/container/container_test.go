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
	"fmt"
	"reflect"
	"testing"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
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
	// NONE should transition to PULLED
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
		t.Run(fmt.Sprintf("IsInternal shoukd return %t for %s", tc.internal, tc.container.String()),
			func(t *testing.T) {
				assert.Equal(t, tc.internal, tc.container.IsInternal())
			})
	}
}

// TestSetupExecutionRoleFlag tests whether or not the container appropriately
//sets the flag for using execution roles
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

func TestSetKnownExecCommandAgentStatus(t *testing.T) {
	container := Container{}
	container.SetKnownExecCommandAgentStatus(apicontainerstatus.ManagedAgentRunning)
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, container.ExecCommandAgentStatusUnsafe)
}

func TestSetExecCommandAgentStartedAt(t *testing.T) {
	container := Container{}
	nowTime := time.Now()
	container.SetExecCommandAgentStartedAt(nowTime)
	assert.Equal(t, nowTime, container.execCommandAgentStartedAt)
}

func TestGetKnownExecCommandAgentStatus(t *testing.T) {
	container := Container{}
	container.SetKnownExecCommandAgentStatus(apicontainerstatus.ManagedAgentRunning)
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, container.GetKnownExecCommandAgentStatus())
}

func TestGetExecCommandAgentStartedAt(t *testing.T) {
	container := Container{}
	nowTime := time.Now()
	container.SetExecCommandAgentStartedAt(nowTime)
	assert.Equal(t, nowTime, container.GetExecCommandAgentStartedAt())
}

func TestGetKnownManagedAgents(t *testing.T) {
	nowTime := time.Now()
	testCases := []struct {
		name              string
		container         *Container
		status            apicontainerstatus.ManagedAgentStatus
		expectedStartTime time.Time
	}{
		{
			name: "test GetKnownManagedAgents none case",
			container: &Container{
				Name: "containerManagedAgentNone",
			},
			status:            apicontainerstatus.ManagedAgentStatusNone,
			expectedStartTime: time.Time{},
		},
		{
			name: "test GetKnownManagedAgents created case",
			container: &Container{
				Name: "containerManagedAgentCreated",
			},
			status:            apicontainerstatus.ManagedAgentCreated,
			expectedStartTime: time.Time{},
		},
		{
			name: "test GetKnownManagedAgents running case",
			container: &Container{
				Name:                      "containerManagedAgentRunning",
				execCommandAgentStartedAt: nowTime,
			},
			status:            apicontainerstatus.ManagedAgentRunning,
			expectedStartTime: nowTime,
		},
		{
			name: "test GetKnownManagedAgents stopped case",
			container: &Container{
				Name:                      "containerManagedAgentStopped",
				execCommandAgentStartedAt: nowTime,
			},
			status:            apicontainerstatus.ManagedAgentStopped,
			expectedStartTime: nowTime,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.container.SetKnownExecCommandAgentStatus(tc.status)

			expectedManagedAgentArray := []ManagedAgent{}
			// only create expected managed agent if ShouldReportToBackend is true
			if tc.status.ShouldReportToBackend() {
				expectedManagedAgent := ManagedAgent{
					Status:        tc.status,
					Name:          executeCommandAgentName,
					LastStartedAt: &tc.expectedStartTime,
				}
				expectedManagedAgentArray = append(expectedManagedAgentArray, expectedManagedAgent)
			}

			assert.Equal(t, expectedManagedAgentArray, tc.container.GetKnownManagedAgents())
		})
	}
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

func TestExecCommandAgentMetadata(t *testing.T) {
	const (
		testPid          = "pid"
		testDockerExecId = "dockerId"
	)
	c := &Container{
		ExecCommandAgentMetadata: &ExecCommandAgentMetadata{},
	}
	assert.Equal(t, "", c.ExecCommandAgentMetadata.PID)
	assert.Equal(t, "", c.ExecCommandAgentMetadata.DockerExecID)

	c.SetExecCommandAgentMetadata(&ExecCommandAgentMetadata{
		PID:          testPid,
		DockerExecID: testDockerExecId,
	})

	assert.Equal(t, testPid, c.ExecCommandAgentMetadata.PID)
	assert.Equal(t, testDockerExecId, c.ExecCommandAgentMetadata.DockerExecID)

	md := c.GetExecCommandAgentMetadata()
	assert.Equal(t, testPid, md.PID)
	assert.Equal(t, testDockerExecId, md.DockerExecID)
}
