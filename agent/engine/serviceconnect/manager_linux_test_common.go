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

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	mockENI = &ni.NetworkInterface{
		ID: "eni-id",
		IPV4Addresses: []*ni.IPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*ni.IPV6Address{
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

func copyMap(input map[string]string, addk string, addv string) map[string]string {
	output := make(map[string]string)
	if len(addk) > 0 && len(addv) > 0 {
		output[addk] = addv
	}
	for k, v := range input {
		output[k] = v
	}
	return output
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

	expectedImage := "container:interface-v1"

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
		"APPNET_ENVOY_LOG_DESTINATION":  "/some/other/log",
	}

	type testCase struct {
		name                 string
		container            *apicontainer.Container
		expectedENV          map[string]string
		expectedBinds        []string
		expectedBindDirPerm  string
		expectedBindDirOwner uint32
		containerInstanceARN string
	}
	testcases := []testCase{
		{
			name:                 "Service connect container has extra binds/ENV. Commercial region has no /etc/pki mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:us-west-2:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        expectedBinds,
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:us-west-2:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. US gov region has no /etc/pki mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:us-gov-west-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        expectedBinds,
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:us-gov-west-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. China region has no /etc/pki mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:cn-north-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        expectedBinds,
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:cn-north-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. Iso region gets extra /etc/pki bind mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:us-iso-east-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        append(expectedBinds, "/etc/pki:/etc/pki"),
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:us-iso-east-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. Iso region gets extra /etc/pki bind mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:eu-isoe-west-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        append(expectedBinds, "/etc/pki:/etc/pki"),
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:eu-isoe-west-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. Iso region gets extra /etc/pki bind mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:us-isof-south-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        append(expectedBinds, "/etc/pki:/etc/pki"),
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:us-isof-south-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. Unknown region gets /etc/pki bind mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "arn:aws:ecs:ap-iso-southeast-1:123456789012:container-instance/12345678-test-test-test-123456789012"),
			expectedBinds:        append(expectedBinds, "/etc/pki:/etc/pki"),
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "arn:aws:ecs:ap-iso-southeast-1:123456789012:container-instance/12345678-test-test-test-123456789012",
		},
		{
			name:                 "Service connect container has extra binds/ENV. Invalid region gets /etc/pki bind mount.",
			container:            serviceConnectContainer,
			expectedENV:          copyMap(expectedENVs, "ECS_CONTAINER_INSTANCE_ARN", "foo-bar-invalid-arn"),
			expectedBinds:        append(expectedBinds, "/etc/pki:/etc/pki"),
			expectedBindDirPerm:  fs.FileMode(0700).String(),
			expectedBindDirOwner: serviceconnect.AppNetUID,
			containerInstanceARN: "foo-bar-invalid-arn",
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

		agentContainerImageName: "container",
		appnetInterfaceVersion:  "v1",

		logPathContainer: "/some/other/log",
		logPathHostRoot:  filepath.Join(tempDir, "log"),
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hostConfig := &dockercontainer.HostConfig{}
			scManager.containerInstanceARN = tc.containerInstanceARN
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig,
				ipcompatibility.NewIPv4OnlyCompatibility())
			if err != nil {
				t.Fatal(err)
			}
			assert.ElementsMatch(t, tc.expectedBinds, hostConfig.Binds)
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

	assert.Equal(t, expectedImage, serviceConnectContainer.Image)
	assert.Equal(t, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"), scTask.ServiceConnectConfig.RuntimeConfig.AdminSocketPath)
	assert.Equal(t, "/give?stats", scTask.ServiceConnectConfig.RuntimeConfig.StatsRequest)
	assert.Equal(t, "/do?drain", scTask.ServiceConnectConfig.RuntimeConfig.DrainRequest)

	config := scTask.GetServiceConnectRuntimeConfig()
	assert.Equal(t, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"), config.AdminSocketPath)
	assert.Equal(t, "/give?stats", config.StatsRequest)
	assert.Equal(t, "/do?drain", config.DrainRequest)
}
