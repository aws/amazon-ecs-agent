//go:build linux && (unit || sudo_unit)
// +build linux
// +build unit sudo_unit

package serviceconnect

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/config"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/aws-sdk-go/aws"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

const (
	ipv4        = "10.0.0.1"
	gatewayIPv4 = "10.0.0.2/20"
	mac         = "1.2.3.4"
	ipv6        = "f0:234:23"
)

var (
	cfg     config.Config
	mockENI = &apieni.ENI{
		ID: "eni-id",
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*apieni.ENIIPV6Address{
			{
				Address: ipv6,
			},
		},
		SubnetGatewayIPV4Address: gatewayIPv4,
	}
)

func mockMkdirAllAndChown(path string, perm fs.FileMode, uid, gid int) error {
	return nil
}

func getAWSVPCTask(t *testing.T) (*apitask.Task, *apicontainer.Container, *apicontainer.Container) {
	sleepTask := testdata.LoadTask("sleep5TwoContainers")

	sleepTask.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: "service-connect",
		DNSConfig: []serviceconnect.DNSConfigEntry{
			{
				HostName: "host1.my.corp",
				Address:  "169.254.1.1",
			},
			{
				HostName: "host1.my.corp",
				Address:  "ff06::c4",
			},
		},
	}
	dockerConfig := dockercontainer.Config{
		Healthcheck: &dockercontainer.HealthConfig{
			Test:     []string{"echo", "ok"},
			Interval: time.Millisecond,
			Timeout:  time.Second,
			Retries:  1,
		},
	}

	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	pauseContainer.Name = apitask.NetworkPauseContainerName
	pauseContainer.Image = fmt.Sprintf("%s:%s", cfg.PauseContainerImageName, cfg.PauseContainerTag)
	pauseContainer.Essential = true
	pauseContainer.Type = apicontainer.ContainerCNIPause

	rawConfig, err := json.Marshal(&dockerConfig)
	if err != nil {
		t.Fatal(err)
	}
	serviceConnectContainer := &apicontainer.Container{
		Name:            sleepTask.ServiceConnectConfig.ContainerName,
		HealthCheckType: apicontainer.DockerHealthCheckType,
		DockerConfig: apicontainer.DockerConfig{
			Config: aws.String(string(rawConfig)),
		},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}
	sleepTask.Containers = append(sleepTask.Containers, serviceConnectContainer)

	// Add eni information to the task so the task can add dependency of pause container
	sleepTask.AddTaskENI(mockENI)
	return sleepTask, pauseContainer, serviceConnectContainer
}

func testAgentContainerModificationsForServiceConnect(t *testing.T, privilegedMode bool) {
	backupMkdirAllAndChown := mkdirAllAndChown
	tempDir := t.TempDir()
	if !privilegedMode {
		mkdirAllAndChown = mockMkdirAllAndChown
	}
	defer func() {
		mkdirAllAndChown = backupMkdirAllAndChown
		os.RemoveAll(tempDir)
	}()
	scTask, _, serviceConnectContainer := getAWSVPCTask(t)

	expectedImage := "container:tag"

	expectedBinds := []string{
		fmt.Sprintf("%s/status/%s:%s", tempDir, scTask.GetID(), "/some/other/run"),
		fmt.Sprintf("%s/relay:%s", tempDir, "/not/var/run"),
		fmt.Sprintf("%s/log/%s:%s", tempDir, scTask.GetID(), "/some/other/log"),
	}
	expectedENVs := map[string]string{
		"ReLaYgOeShErE":                 "unix:///not/var/run/relay_file_of_holiness",
		"StAtUsGoEsHeRe":                "/some/other/run/status_file_of_holiness",
		"APPNET_AGENT_ADMIN_MODE":       "uds",
		"ENVOY_ENABLE_IAM_AUTH_FOR_XDS": "0",
		"ECS_CONTAINER_INSTANCE_ARN":    "fake_container_instance",
		"APPNET_ENVOY_LOG_DESTINATION":  "/some/other/log",
	}

	type testCase struct {
		name                 string
		container            *apicontainer.Container
		expectedENV          map[string]string
		expectedBinds        []string
		expectedBindDirPerm  string
		expectedBindDirOwner uint32
	}
	testcases := []testCase{
		{
			name:                 "Service connect container has extra binds/ENV",
			container:            serviceConnectContainer,
			expectedENV:          expectedENVs,
			expectedBinds:        expectedBinds,
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
		},
	}
	// Add test cases for other containers expecting no modifications
	for _, container := range scTask.Containers {
		if container != serviceConnectContainer {
			testcases = append(testcases, testCase{name: container.Name, container: container, expectedENV: map[string]string{}})
		}
	}
	scManager := &manager{
		relayPathContainer:  "/not/var/run",
		relayPathHost:       filepath.Join(tempDir, "relay"),
		relayFileName:       "relay_file_of_holiness",
		endpointENV:         "ReLaYgOeShErE",
		statusPathContainer: "/some/other/run",
		statusPathHostRoot:  filepath.Join(tempDir, "status"),
		statusFileName:      "status_file_of_holiness",
		statusENV:           "StAtUsGoEsHeRe",
		adminStatsRequest:   "/give?stats",
		adminDrainRequest:   "/do?drain",

		AgentContainerImageName: "container",
		AgentContainerTag:       "tag",

		containerInstanceARN: "fake_container_instance",
		logPathContainer:     "/some/other/log",
		logPathHostRoot:      filepath.Join(tempDir, "log"),
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hostConfig := &dockercontainer.HostConfig{}
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.expectedBinds, hostConfig.Binds)
			assert.Equal(t, tc.expectedENV, tc.container.Environment)
			if privilegedMode {
				for _, bind := range hostConfig.Binds {
					hostDir := strings.Split(bind, ":")[0]
					dirStat, err := os.Stat(hostDir)
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedBindDirPerm, dirStat.Mode().Perm().String(),
						fmt.Sprintf("directory %s should have mode %s", hostDir, tc.expectedBindDirPerm))
					assert.Equal(t, tc.expectedBindDirOwner, dirStat.Sys().(*syscall.Stat_t).Uid)
				}
			}
		})
	}

	assert.Equal(t, serviceConnectContainer.Image, expectedImage)
	assert.Equal(t, scTask.ServiceConnectConfig.RuntimeConfig.AdminSocketPath, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"))
	assert.Equal(t, scTask.ServiceConnectConfig.RuntimeConfig.StatsRequest, "/give?stats")
	assert.Equal(t, scTask.ServiceConnectConfig.RuntimeConfig.DrainRequest, "/do?drain")

	config := scTask.GetServiceConnectRuntimeConfig()
	assert.Equal(t, config.AdminSocketPath, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"))
	assert.Equal(t, config.StatsRequest, "/give?stats")
	assert.Equal(t, config.DrainRequest, "/do?drain")
}
