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
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"

	"github.com/docker/go-connections/nat"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_secretsmanageriface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/service/secretsmanager"

	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/envFiles"
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
	serviceConnectContainerTestName = "service-connect"
	testHostName                    = "testHostName"
	testOutboundListenerName        = "testOutboundListener"
	testIPv4Address                 = "172.31.21.40"
	testIPv6Address                 = "abcd:dcba:1234:4321::"
	testIPv4Cidr                    = "127.255.0.0/16"
	testIPv6Cidr                    = "2002::1234:abcd:ffff:c0a8:101/64"
)

var (
	testListenerPort              = uint16(8080)
	testBridgeDefaultListenerPort = uint16(15000)
)

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

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	config, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

var (
	SCTaskContainerPort1            uint16 = 8080
	SCTaskContainerPort2            uint16 = 9090
	SCIngressListener1ContainerPort uint16 = 15000
	SCIngressListener2ContainerPort uint16 = 16000
	SCIngressListener2HostPort      uint16 = 17000
	SCEgressListenerContainerPort   uint16 = 12345
	defaultSCProtocol                      = "/tcp"
)

func getTestTaskServiceConnectBridgeMode() *Task {
	testTask := &Task{
		NetworkMode: BridgeNetworkMode,
		Containers: []*apicontainer.Container{
			{
				Name: "C1",
				Ports: []apicontainer.PortBinding{
					{SCTaskContainerPort1, 0, "", apicontainer.TransportProtocolTCP},
					{SCTaskContainerPort2, 0, "", apicontainer.TransportProtocolTCP},
				},
				NetworkModeUnsafe: "", // should later be overridden to container mode
			},
			{
				Name:              fmt.Sprintf("%s-%s", NetworkPauseContainerName, "C1"),
				Type:              apicontainer.ContainerCNIPause,
				NetworkModeUnsafe: "", // should later be overridden to explicit bridge mode
			},
			{
				Name:              serviceConnectContainerTestName, // port binding is retrieved through listener config and published by pause container
				NetworkModeUnsafe: "",                              // should later be overridden to container mode
			},
			{
				Name:              fmt.Sprintf("%s-%s", NetworkPauseContainerName, serviceConnectContainerTestName),
				Type:              apicontainer.ContainerCNIPause,
				NetworkModeUnsafe: "", // should later be overridden to explicit bridge mode
			},
		},
	}

	testTask.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: serviceConnectContainerTestName,
		IngressConfig: []serviceconnect.IngressConfigEntry{
			{
				ListenerName: "testListener1", // bridge mode default - ephemeral listener host port
				ListenerPort: SCIngressListener1ContainerPort,
			},
			{
				ListenerName: "testListener2", // bridge mode non-default - user-specified listener host port
				ListenerPort: SCIngressListener2ContainerPort,
				HostPort:     &SCIngressListener2HostPort,
			},
		},
		EgressConfig: &serviceconnect.EgressConfig{
			ListenerName: "testEgressListener",
			ListenerPort: SCEgressListenerContainerPort, // Presently this should always get ephemeral port
		},
	}
	return testTask
}

func convertSCPort(port uint16) nat.Port {
	return nat.Port(strconv.Itoa(int(port)) + defaultSCProtocol)
}

// TestDockerHostConfigSCBridgeMode verifies port bindings and network mode overrides for each
// container in an SC-enabled bridge mode task. The test task is consisted of the SC container, a regular container,
// and two pause containers associated with each.
func TestDockerHostConfigSCBridgeMode(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	// task container and SC container should both get empty port binding map and "container" network mode
	actualConfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Equal(t, dockercontainer.NetworkMode(fmt.Sprintf("%s-%s", // e.g. "container:dockerid-~internal~ecs~pause-C1"
		dockerMappingContainerPrefix+dockerIDPrefix+NetworkPauseContainerName, "C1")), actualConfig.NetworkMode)
	assert.Empty(t, actualConfig.PortBindings, "Task container port binding should be empty")

	actualConfig, err = testTask.DockerHostConfig(testTask.Containers[2], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Equal(t, dockercontainer.NetworkMode(fmt.Sprintf("%s-%s", // e.g. "container:dockerid-~internal~ecs~pause-C1"
		dockerMappingContainerPrefix+dockerIDPrefix+NetworkPauseContainerName, serviceConnectContainerTestName)), actualConfig.NetworkMode)
	assert.Empty(t, actualConfig.PortBindings, "SC container port binding should be empty")

	// task pause container should get port binding map of the task container
	actualConfig, err = testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Equal(t, dockercontainer.NetworkMode(BridgeNetworkMode), actualConfig.NetworkMode)
	bindings, ok := actualConfig.PortBindings[convertSCPort(SCTaskContainerPort1)]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "0", bindings[0].HostPort, "Wrong hostport")
	bindings, ok = actualConfig.PortBindings[convertSCPort(SCTaskContainerPort2)]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "0", bindings[0].HostPort, "Wrong hostport")

	// SC pause container should get port binding map of all ingress listeners
	actualConfig, err = testTask.DockerHostConfig(testTask.Containers[3], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Equal(t, dockercontainer.NetworkMode(BridgeNetworkMode), actualConfig.NetworkMode)
	// SC - ingress listener 1 - default experience
	bindings, ok = actualConfig.PortBindings[convertSCPort(SCIngressListener1ContainerPort)]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, "0", bindings[0].HostPort, "Wrong hostport")
	// SC - ingress listener 2 - non-default host port
	bindings, ok = actualConfig.PortBindings[convertSCPort(SCIngressListener2ContainerPort)]
	assert.True(t, ok, "Could not get port bindings")
	assert.Equal(t, 1, len(bindings), "Wrong number of bindings")
	assert.Equal(t, strconv.Itoa(int(SCIngressListener2HostPort)), bindings[0].HostPort, "Wrong hostport")
	// SC - egress listener - should not have port binding
	bindings, ok = actualConfig.PortBindings[convertSCPort(SCEgressListenerContainerPort)]
	assert.False(t, ok, "egress listener has port binding but it shouldn't")
}

// TestDockerHostConfigSCBridgeMode_getPortBindingFailure verifies that when we can't find the task
// container associated with the pause container, DockerHostConfig should return failure (from getPortBinding)
func TestDockerHostConfigSCBridgeMode_getPortBindingFailure(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.Containers[1].Name = "invalid" // make the pause container name invalid such that we can't resolve task container from it
	_, err := testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Msg, "error retrieving docker port map"))
}

// TestDockerContainerConfigSCBridgeMode verifies exposed port and uid configuration for each container
// in an SC-enabled bridge mode task. The test task is consisted of the SC container, a regular container,
// and two pause container associated with each of them.
func TestDockerContainerConfigSCBridgeMode(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()

	// Containers[0] aka user-defined task container should NOT expose any ports (it's done through the associated pause container)
	// It should NOT get UID override
	actualConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Empty(t, actualConfig.ExposedPorts)
	assert.Empty(t, actualConfig.User)

	// Containers[2] aka SC container should NOT expose any ports (it's done through the associated pause container)
	// It should get UID override
	actualConfig, err = testTask.DockerConfig(testTask.Containers[2], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.Empty(t, actualConfig.ExposedPorts)
	assert.Equal(t, strconv.Itoa(serviceconnect.AppNetUID), actualConfig.User)

	// Containers[1] aka task pause container should expose all container ports from the associated user-defined task containers
	// It should NOT get UID override
	actualConfig, err = testTask.DockerConfig(testTask.Containers[1], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.NotNil(t, actualConfig.ExposedPorts)
	assert.Equal(t, 2, len(actualConfig.ExposedPorts))
	_, ok := actualConfig.ExposedPorts[convertSCPort(SCTaskContainerPort1)]
	assert.True(t, ok)
	_, ok = actualConfig.ExposedPorts[convertSCPort(SCTaskContainerPort2)]
	assert.True(t, ok)
	assert.Empty(t, actualConfig.User)

	// Containers[3] aka SC pause container should expose all container ports from SC ingress and egress listeners
	// It should NOT get UID override
	actualConfig, err = testTask.DockerConfig(testTask.Containers[3], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.NotNil(t, actualConfig.ExposedPorts)
	assert.Equal(t, 3, len(actualConfig.ExposedPorts))
	_, ok = actualConfig.ExposedPorts[convertSCPort(SCIngressListener1ContainerPort)]
	assert.True(t, ok)
	_, ok = actualConfig.ExposedPorts[convertSCPort(SCIngressListener2ContainerPort)]
	assert.True(t, ok)
	_, ok = actualConfig.ExposedPorts[convertSCPort(SCEgressListenerContainerPort)]
	assert.True(t, ok)
	assert.Empty(t, actualConfig.User)
}

func TestDockerContainerConfigSCBridgeMode_getExposedPortsFailure(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.Containers[1].Name = "invalid" // make the pause container name invalid such that we can't resolve task container from it
	_, err := testTask.DockerConfig(testTask.Containers[1], defaultDockerClientAPIVersion)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Msg, "error resolving docker exposed ports"))
}

func TestDockerContainerConfigSCBridgeMode_emptyEgressConfig(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.ServiceConnectConfig.EgressConfig = nil
	actualConfig, err := testTask.DockerConfig(testTask.Containers[3], defaultDockerClientAPIVersion)
	assert.Nil(t, err)
	assert.NotNil(t, actualConfig)
	assert.NotNil(t, actualConfig.ExposedPorts)
	assert.Equal(t, 2, len(actualConfig.ExposedPorts))
	_, ok := actualConfig.ExposedPorts[convertSCPort(SCIngressListener1ContainerPort)]
	assert.True(t, ok)
	_, ok = actualConfig.ExposedPorts[convertSCPort(SCIngressListener2ContainerPort)]
	assert.True(t, ok)
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

	config, err := testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	config, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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
		ENIs: []*apieni.ENI{
			{
				ID: "eniID",
			},
		},
		NetworkMode: AWSVPCNetworkMode,
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
	cfg, err := testTask.DockerHostConfig(customContainer, dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, "container:"+dockerIDPrefix+NetworkPauseContainerName, string(cfg.NetworkMode))

	// Verify that the network mode is not set to "none"  for the
	// empty volume container
	cfg, err = testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, networkModeNone, string(cfg.NetworkMode))

	// Verify that the network mode is set to "none" for the pause container
	cfg, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, networkModeNone, string(cfg.NetworkMode))

	// Verify that overridden DNS settings are set for the pause container
	// and not set for non pause containers
	testTask.ENIs[0].DomainNameServers = []string{"169.254.169.253"}
	testTask.ENIs[0].DomainNameSearchList = []string{"us-west-2.compute.internal"}

	// DNS overrides are only applied to the pause container. Verify that the non-pause
	// container contains no overrides
	cfg, err = testTask.DockerHostConfig(customContainer, dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(cfg.DNS))
	assert.Equal(t, 0, len(cfg.DNSSearch))

	// Verify DNS settings are overridden for the pause container
	cfg, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, []string{"169.254.169.253"}, cfg.DNS)
	assert.Equal(t, []string{"us-west-2.compute.internal"}, cfg.DNSSearch)

	// Verify eni ExtraHosts  added to HostConfig for pause container
	ipaddr := &apieni.ENIIPV4Address{Primary: true, Address: "10.0.1.1"}
	testTask.ENIs[0].IPV4Addresses = []*apieni.ENIIPV4Address{ipaddr}
	testTask.ENIs[0].PrivateDNSName = "eni.ip.region.compute.internal"

	testTask.ENIs[0].IPV6Addresses = []*apieni.ENIIPV6Address{{Address: ipv6}}
	cfg, err = testTask.DockerHostConfig(pauseContainer, dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)
	assert.Equal(t, []string{"eni.ip.region.compute.internal:10.0.1.1"}, cfg.ExtraHosts)

	// Verify ipv6 setting is enabled.
	if runtime.GOOS == "linux" {
		require.NotNil(t, cfg.Sysctls)
		assert.Equal(t, sysctlValueOff, cfg.Sysctls[disableIPv6SysctlKey])
	}

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
		_, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(&testTask), defaultDockerClientAPIVersion,
			&config.Config{})
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
	cfg := &config.Config{}
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
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
	cfg := &config.Config{}
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
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
	cfg := &config.Config{}
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
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
	cfg := &config.Config{}
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(0), resources.Memory, "Wrong amount of memory")
}

func TestGetDockerResourcesExternalGPUInstance(t *testing.T) {
	container := &apicontainer.Container{
		Name:   "c1",
		CPU:    uint(10),
		Memory: uint(256),
		GPUIDs: []string{"gpu1"},
	}
	testTask := &Task{
		Arn:        "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:     "myFamily",
		Version:    "1",
		Containers: []*apicontainer.Container{container},
	}
	cfg := &config.Config{
		GPUSupportEnabled: true,
	}
	cfg.External.Value = config.ExplicitlyEnabled
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(268435456), resources.Memory, "Wrong amount of memory")
	assert.Equal(t, resources.DeviceRequests[0].DeviceIDs, container.GPUIDs, "Wrong GPU IDs assigned")
}

func TestGetDockerResourcesInternalGPUInstance(t *testing.T) {
	container := &apicontainer.Container{
		Name:   "c1",
		CPU:    uint(10),
		Memory: uint(256),
		GPUIDs: []string{"gpu1"},
	}
	testTask := &Task{
		Arn:        "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:     "myFamily",
		Version:    "1",
		Containers: []*apicontainer.Container{container},
	}
	cfg := &config.Config{
		GPUSupportEnabled: true,
	}
	resources := testTask.getDockerResources(testTask.Containers[0], cfg)
	assert.Equal(t, int64(10), resources.CPUShares, "Wrong number of CPUShares")
	assert.Equal(t, int64(268435456), resources.Memory, "Wrong amount of memory")
	assert.Equal(t, int64(len(resources.DeviceRequests)), int64(0), "GPU IDs to be handled by env var for internal instance")
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

// Test that the PostUnmarshal function properly changes EfsVolumeConfiguration
// task definitions into a dockerVolumeConfiguration task resource.
func TestPostUnmarshalTaskWithEFSVolumes(t *testing.T) {
	taskFromACS := getACSEFSTask()
	seqNum := int64(42)

	testCases := map[string]string{
		"us-west-2":     "fs-12345.efs.us-west-2.amazonaws.com",
		"cn-north-1":    "fs-12345.efs.cn-north-1.amazonaws.com.cn",
		"us-iso-east-1": "fs-12345.efs.us-iso-east-1.c2s.ic.gov",
		"not-a-region":  "fs-12345.efs.not-a-region.amazonaws.com",
	}
	for region, expectedHostname := range testCases {
		t.Run(region, func(t *testing.T) {
			task, err := TaskFromACS(taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
			assert.Nil(t, err, "Should be able to handle acs task")
			assert.Equal(t, 1, len(task.Containers)) // before PostUnmarshalTask
			cfg := config.Config{}
			cfg.AWSRegion = region
			require.NoError(t, task.PostUnmarshalTask(&cfg, nil, nil, nil, nil))
			assert.Equal(t, 1, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
			assert.Equal(t, 1, len(task.Volumes), "Should have 1 volume")
			taskVol := task.Volumes[0]
			assert.Equal(t, "efsvolume", taskVol.Name)
			assert.Equal(t, "efs", taskVol.Type)

			resources := task.GetResources()
			assert.Len(t, resources, 1)
			vol, ok := resources[0].(*taskresourcevolume.VolumeResource)
			require.True(t, ok)
			dockerVolName := vol.VolumeConfig.DockerVolumeName
			b, err := json.Marshal(resources[0])
			require.NoError(t, err)
			assert.JSONEq(t, fmt.Sprintf(`{
		"name": "efsvolume",
		"dockerVolumeConfiguration": {
		  "scope": "task",
		  "autoprovision": false,
		  "mountPoint": "",
		  "driver": "local",
		  "driverOpts": {
			"device": ":/tmp",
			"o": "addr=%s,nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport",
			"type": "nfs"
		  },
		  "labels": {},
		  "dockerVolumeName": "%s"
		},
		"createdAt": "0001-01-01T00:00:00Z",
		"desiredStatus": "NONE",
		"knownStatus": "NONE"
	  }`, expectedHostname, dockerVolName), string(b))
		})
	}
}

func TestPostUnmarshalTaskWithEFSVolumesThatUseECSVolumePlugin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testCreds := getACSIAMRoleCredentials()
	credentialsIDInTask := aws.StringValue(testCreds.CredentialsId)
	credentialsManager := mock_credentials.NewMockManager(ctrl)
	taskCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: credentialsIDInTask},
	}
	expectedEndpoint := "/v2/credentials/" + credentialsIDInTask
	credentialsManager.EXPECT().GetTaskCredentials(credentialsIDInTask).Return(taskCredentials, true)

	taskFromACS := getACSEFSTask()
	taskFromACS.RoleCredentials = testCreds
	seqNum := int64(42)
	task, err := TaskFromACS(taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 1, len(task.Containers))
	task.SetCredentialsID(aws.StringValue(testCreds.CredentialsId))

	cfg := &config.Config{}
	cfg.VolumePluginCapabilities = []string{"efsAuth"}
	require.NoError(t, task.PostUnmarshalTask(cfg, credentialsManager, nil, nil, nil))
	assert.Equal(t, 1, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
	assert.Equal(t, 1, len(task.Volumes), "Should have 1 volume")
	taskVol := task.Volumes[0]
	assert.Equal(t, "efsvolume", taskVol.Name)
	assert.Equal(t, "efs", taskVol.Type)

	resources := task.GetResources()
	assert.Len(t, resources, 1)
	vol, ok := resources[0].(*taskresourcevolume.VolumeResource)
	require.True(t, ok)
	dockerVolName := vol.VolumeConfig.DockerVolumeName
	b, err := json.Marshal(resources[0])
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"name": "efsvolume",
		"dockerVolumeConfiguration": {
		  "scope": "task",
		  "autoprovision": false,
		  "mountPoint": "",
		  "driver": "amazon-ecs-volume-plugin",
		  "driverOpts": {
			"device": "fs-12345:/tmp",
			"o": "tls,tlsport=12345,iam,awscredsuri=%s,accesspoint=fsap-123",
			"type": "efs"
		  },
		  "labels": {},
		  "dockerVolumeName": "%s"
		},
		"createdAt": "0001-01-01T00:00:00Z",
		"desiredStatus": "NONE",
		"knownStatus": "NONE"
	  }`, expectedEndpoint, dockerVolName), string(b))
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

func TestInitializeContainersV4MetadataEndpoint(t *testing.T) {
	task := Task{
		Containers: []*apicontainer.Container{
			{
				Name:        "c1",
				Environment: make(map[string]string),
			},
		},
	}
	container := task.Containers[0]

	task.initializeContainersV4MetadataEndpoint(utils.NewStaticUUIDProvider("new-uuid"))

	// Test if the v3 endpoint id is set and the endpoint is injected to env
	assert.Equal(t, container.GetV3EndpointID(), "new-uuid")
	assert.Equal(t, container.Environment[apicontainer.MetadataURIEnvVarNameV4],
		fmt.Sprintf(apicontainer.MetadataURIFormatV4, "new-uuid"))
}

// Tests that task.initializeContainersV1AgentAPIEndpoint method initializes
// V3EndpointID for all containers of the task and injects v1 Agent API Endpoint
// as an environment variable into each container.
func TestInitializeContainersV1AgentAPIEndpoint(t *testing.T) {
	// Create a dummy task
	task := Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
			},
			{
				Name: "c2",
			},
		},
	}

	// Call the method
	task.initializeContainersV1AgentAPIEndpoint(utils.NewStaticUUIDProvider("new-uuid"))

	// Assert that v3 endpoint id is set and the endpoint is injected to env of each container
	for _, container := range task.Containers {
		assert.Equal(t, "new-uuid", container.GetV3EndpointID())
		assert.Equal(t,
			map[string]string{
				apicontainer.AgentURIEnvVarName: "http://169.254.170.2/api/new-uuid",
			},
			container.Environment)
	}
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
				dockHostCfg, err := task.DockerHostConfig(container, docMaps, defaultDockerClientAPIVersion,
					&config.Config{})
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
				dockHostCfg, err := task.DockerHostConfig(container, docMaps, defaultDockerClientAPIVersion,
					&config.Config{})
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
		ServiceName:   strptr("myService"),
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
				EnvironmentFiles: []*ecsacs.EnvironmentFile{
					{
						Value: strptr("s3://bucketName/envFile"),
						Type:  strptr("s3"),
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
		RoleCredentials: getACSIAMRoleCredentials(),
		Cpu:             floatptr(2.0),
		Memory:          intptr(512),
	}
	expectedTask := &Task{
		Arn:                 "myArn",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Family:              "myFamily",
		Version:             "1",
		ServiceName:         "myService",
		NetworkMode:         BridgeNetworkMode,
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
				EnvironmentFiles: []apicontainer.EnvironmentFile{
					{
						Value: "s3://bucketName/envFile",
						Type:  "s3",
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
	expectedTask.GetID() // to set the task setIdOnce (sync.Once) property

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

// TestGetIDHappyPath validates the happy path of GetID
func TestGetIDHappyPath(t *testing.T) {
	taskNormalARN := Task{
		Arn: "arn:aws:ecs:region:account-id:task/task-id",
	}
	taskLongARN := Task{
		Arn: "arn:aws:ecs:region:account-id:task/cluster-name/task-id",
	}

	taskID := taskNormalARN.GetID()
	assert.Equal(t, "task-id", taskID)

	taskID = taskLongARN.GetID()
	assert.Equal(t, "task-id", taskID)
}

// TestTaskGetPrimaryENI tests the eni can be correctly acquired by calling GetTaskPrimaryENI
func TestTaskGetPrimaryENI(t *testing.T) {
	enisOfTask := []*apieni.ENI{
		{
			ID: "id",
		},
	}
	testTask := &Task{
		ENIs: enisOfTask,
	}

	eni := testTask.GetPrimaryENI()
	assert.NotNil(t, eni)
	assert.Equal(t, "id", eni.ID)

	testTask.ENIs = nil
	eni = testTask.GetPrimaryENI()
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
	require.NoError(t, err)

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

	cfg, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)

	err = task.ApplyExecutionRoleLogsAuth(cfg, credentialsManager)
	assert.Nil(t, err)

	endpoint, ok := cfg.LogConfig.Config["awslogs-credentials-endpoint"]
	assert.True(t, ok)
	assert.Equal(t, expectedEndpoint, endpoint)
}

func TestApplyExecutionRoleLogsAuthNoConfigInHostConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	credentialsIDInTask := "credsid"
	expectedEndpoint := "/v2/credentials/" + credentialsIDInTask

	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: "foo",
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	require.NoError(t, err)

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

	cfg, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)

	err = task.ApplyExecutionRoleLogsAuth(cfg, credentialsManager)
	assert.Nil(t, err)

	endpoint, ok := cfg.LogConfig.Config["awslogs-credentials-endpoint"]
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
	require.NoError(t, err)

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

	cfg, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)

	err = task.ApplyExecutionRoleLogsAuth(cfg, credentialsManager)
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
	require.NoError(t, err)

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

	cfg, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion, &config.Config{})
	assert.Error(t, err)

	err = task.ApplyExecutionRoleLogsAuth(cfg, credentialsManager)
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

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Nil(t, err)

	assert.Equal(t, int64(apicontainer.DockerContainerMinimumMemoryInBytes), hostconfig.Memory)

	hostconfig, err = testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), dockerclient.Version_1_18,
		&config.Config{})
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
		Arn:                testTaskARN,
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
		Arn:                testTaskARN,
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
		Arn:                testTaskARN,
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
	configMap := map[string]string{
		"splunk-option": "option",
	}
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
	assert.Equal(t, "option", hostConfig.LogConfig.Config["splunk-option"])
}

func TestPopulateSecretsNoConfigInHostConfig(t *testing.T) {
	secret1 := apicontainer.Secret{
		Provider:  "ssm",
		Name:      "splunk-token",
		Region:    "us-west-1",
		Target:    "LOG_DRIVER",
		ValueFrom: "/test/secretName1",
	}

	container := &apicontainer.Container{
		Name:                      "myName",
		Image:                     "image:tag",
		Secrets:                   []apicontainer.Secret{secret1},
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

	ssmRes := &ssmsecret.SSMSecretResource{}
	ssmRes.SetCachedSecretValue(secKeyLogDriver, "secretValue1")
	task.AddResource(ssmsecret.ResourceName, ssmRes)
	task.PopulateSecrets(hostConfig, container)
	assert.Equal(t, "secretValue1", hostConfig.LogConfig.Config["splunk-token"])
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

	cfg := &config.Config{GPUSupportEnabled: true}
	err := task.addGPUResource(cfg)

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
	cfg := &config.Config{GPUSupportEnabled: true}
	err := task.addGPUResource(cfg)
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

	cfg := &config.Config{GPUSupportEnabled: true, NvidiaRuntime: config.DefaultNvidiaRuntime}
	testTask.addGPUResource(cfg)
	dockerHostConfig, _ := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	cfg := &config.Config{GPUSupportEnabled: true}
	testTask.addGPUResource(cfg)
	dockerHostConfig, _ := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
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

	cfg := &config.Config{GPUSupportEnabled: true}
	testTask.addGPUResource(cfg)
	_, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{})
	assert.Error(t, err)
}

func TestDockerHostConfigNeuronRuntime(t *testing.T) {
	testTask := &Task{
		Arn: "test",
		Containers: []*apicontainer.Container{
			{
				Name:  "myName1",
				Image: "image:tag",
				Environment: map[string]string{
					"AWS_NEURON_VISIBLE_DEVICES": "all",
				},
			},
		},
	}

	dockerHostConfig, _ := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), defaultDockerClientAPIVersion,
		&config.Config{InferentiaSupportEnabled: true})
	assert.Equal(t, neuronRuntime, dockerHostConfig.Runtime)
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

	err := task.initializeContainerOrdering()
	assert.NoError(t, err)

	containerResultWithVolume := task.Containers[0]
	assert.Equal(t, "myName1", containerResultWithVolume.DependsOnUnsafe[0].ContainerName)
	assert.Equal(t, ContainerOrderingCreateCondition, containerResultWithVolume.DependsOnUnsafe[0].Condition)

	containerResultWithLink := task.Containers[1]
	assert.Equal(t, "myName", containerResultWithLink.DependsOnUnsafe[0].ContainerName)
	assert.Equal(t, ContainerOrderingStartCondition, containerResultWithLink.DependsOnUnsafe[0].Condition)

	containerResultWithBothVolumeAndLink := task.Containers[2]
	assert.Equal(t, "myName", containerResultWithBothVolumeAndLink.DependsOnUnsafe[0].ContainerName)
	assert.Equal(t, ContainerOrderingCreateCondition, containerResultWithBothVolumeAndLink.DependsOnUnsafe[0].Condition)
	assert.Equal(t, "myName1", containerResultWithBothVolumeAndLink.DependsOnUnsafe[1].ContainerName)
	assert.Equal(t, ContainerOrderingStartCondition, containerResultWithBothVolumeAndLink.DependsOnUnsafe[1].Condition)

	containerResultWithNoVolumeOrLink := task.Containers[3]
	assert.Equal(t, 0, len(containerResultWithNoVolumeOrLink.DependsOnUnsafe))
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

	task1v := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{containerWithVolumeError},
	}

	task1l := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{containerWithLinkError1},
	}

	task2l := &Task{
		Arn:                "test",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{containerWithLinkError2},
	}

	errVolume1 := task1v.initializeContainerOrdering()
	assert.Error(t, errVolume1)
	errLink1 := task1l.initializeContainerOrdering()
	assert.Error(t, errLink1)

	errLink2 := task2l.initializeContainerOrdering()
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

// Tests that ACS Task to Task translation does not fail when ServiceName is missing.
// Asserts that Task.ServiceName is empty in such a case.
func TestTaskFromACSServiceNameMissing(t *testing.T) {
	taskFromACS := ecsacs.Task{} // No service name
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, task.ServiceName, "")
}

func TestGetContainerIndex(t *testing.T) {
	task := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
			},
			{
				Name: "p",
				Type: apicontainer.ContainerCNIPause,
			},
			{
				Name: "c2",
			},
		},
	}

	assert.Equal(t, 0, task.GetContainerIndex("c1"))
	assert.Equal(t, 1, task.GetContainerIndex("c2"))
	assert.Equal(t, -1, task.GetContainerIndex("p"))
}

func TestPostUnmarshalTaskEnvfiles(t *testing.T) {
	envfile := apicontainer.EnvironmentFile{
		Value: "s3://bucket/envfile",
		Type:  "s3",
	}

	container := &apicontainer.Container{
		Name:                      "containerName",
		Image:                     "image:tag",
		EnvironmentFiles:          []apicontainer.EnvironmentFile{envfile},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                testTaskARN,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{}
	credentialsManager := mock_credentials.NewMockManager(ctrl)
	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			CredentialsManager: credentialsManager,
		},
	}

	resourceDep := apicontainer.ResourceDependency{
		Name:           envFiles.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(envFiles.EnvFileCreated),
	}

	err := task.PostUnmarshalTask(cfg, credentialsManager, resFields, nil, nil)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(task.ResourcesMapUnsafe))
	assert.Equal(t, resourceDep,
		task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])
}

func TestInitializeAndGetEnvfilesResource(t *testing.T) {
	envfile := apicontainer.EnvironmentFile{
		Value: "s3://bucket/envfile",
		Type:  "s3",
	}

	container := &apicontainer.Container{
		Name:                      "containerName",
		Image:                     "image:tag",
		EnvironmentFiles:          []apicontainer.EnvironmentFile{envfile},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "testArn",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{
		DataDir: "/ecs/data",
	}
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	task.initializeEnvfilesResource(cfg, credentialsManager)

	resourceDep := apicontainer.ResourceDependency{
		Name:           envFiles.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(envFiles.EnvFileCreated),
	}

	assert.Equal(t, resourceDep,
		task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])

	_, ok := task.getEnvfilesResource("containerName")
	assert.True(t, ok)
}

func TestRequiresEnvfiles(t *testing.T) {
	envfile := apicontainer.EnvironmentFile{
		Value: "s3://bucket/envfile",
		Type:  "s3",
	}

	container := &apicontainer.Container{
		Name:                      "containerName",
		Image:                     "image:tag",
		EnvironmentFiles:          []apicontainer.EnvironmentFile{envfile},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &Task{
		Arn:                "testArn",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers:         []*apicontainer.Container{container},
	}

	assert.Equal(t, true, requireEnvfiles(task))
}

func requireEnvfiles(task *Task) bool {
	for _, container := range task.Containers {
		if container.ShouldCreateWithEnvFiles() {
			return true
		}
	}
	return false
}

func TestPopulateTaskARN(t *testing.T) {
	task := &Task{
		Arn: testTaskARN,
		Containers: []*apicontainer.Container{
			{
				Name: "test",
			},
		},
	}
	task.populateTaskARN()
	assert.Equal(t, task.Arn, task.Containers[0].GetTaskARN())
}

func TestShouldEnableIPv6(t *testing.T) {
	task := &Task{
		ENIs: []*apieni.ENI{getTestENI()},
	}
	assert.True(t, task.shouldEnableIPv6())
	task.ENIs[0].IPV6Addresses = nil
	assert.False(t, task.shouldEnableIPv6())
}

func getTestENI() *apieni.ENI {
	return &apieni.ENI{
		ID: "test",
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
		SubnetGatewayIPV4Address: ipv4Gateway + ipv4Block,
	}
}

func TestPostUnmarshalTaskWithOptions(t *testing.T) {
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	numCalls := 0
	opt := func(optTask *Task) error {
		assert.Equal(t, task, optTask)
		numCalls++
		return nil
	}
	task.PostUnmarshalTask(&config.Config{}, nil, nil, nil, nil, opt, opt)
	assert.Equal(t, 2, numCalls)
}

func TestGetServiceConnectContainer(t *testing.T) {
	const serviceConnectContainerName = "service-connect"
	scContainer := &apicontainer.Container{
		Name: serviceConnectContainerName,
	}
	tt := []struct {
		scConfig *serviceconnect.Config
	}{
		{
			scConfig: nil,
		},
		{
			scConfig: &serviceconnect.Config{
				ContainerName: serviceConnectContainerName,
			},
		},
	}
	for _, tc := range tt {
		task := &Task{
			ServiceConnectConfig: tc.scConfig,
			Containers: []*apicontainer.Container{
				scContainer,
			},
		}
		c := task.GetServiceConnectContainer()
		if tc.scConfig == nil {
			assert.Nil(t, c)
		} else {
			assert.Equal(t, scContainer, c)
		}
	}
}

func TestIsServiceConnectEnabled(t *testing.T) {
	const serviceConnectContainerName = "service-connect"
	tt := []struct {
		scConfig          *serviceconnect.Config
		scContainer       *apicontainer.Container
		expectedSCEnabled bool
	}{
		{
			scConfig:          nil,
			scContainer:       nil,
			expectedSCEnabled: false,
		},
		{
			scConfig: &serviceconnect.Config{
				ContainerName: serviceConnectContainerName,
			},
			scContainer:       nil,
			expectedSCEnabled: false,
		},
		{
			scConfig: nil,
			scContainer: &apicontainer.Container{
				Name: serviceConnectContainerName,
			},
			expectedSCEnabled: false,
		},
		{
			scConfig: &serviceconnect.Config{
				ContainerName: serviceConnectContainerName,
			},
			scContainer: &apicontainer.Container{
				Name: serviceConnectContainerName,
			},
			expectedSCEnabled: true,
		},
	}

	for _, tc := range tt {
		task := &Task{
			ServiceConnectConfig: tc.scConfig,
		}
		if tc.scContainer != nil {
			task.Containers = append(task.Containers, tc.scContainer)
		}
		assert.Equal(t, tc.expectedSCEnabled, task.IsServiceConnectEnabled())
	}
}

func TestPostUnmarshalTaskWithServiceConnectAWSVPCMode(t *testing.T) {
	const (
		utilizedPort1 = 33333
		utilizedPort2 = 44444
		utilizedPort3 = 55555
	)
	utilizedPorts := map[uint16]struct{}{
		utilizedPort1: {},
		utilizedPort2: {},
		utilizedPort3: {},
	}

	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			containerFromACS("C1", utilizedPort1, 0, AWSVPCNetworkMode),
			containerFromACS("C2", utilizedPort2, 0, AWSVPCNetworkMode),
			containerFromACS(serviceConnectContainerTestName, 0, 0, AWSVPCNetworkMode),
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	testSCConfig := serviceconnect.Config{
		ContainerName: serviceConnectContainerTestName,
		IngressConfig: []serviceconnect.IngressConfigEntry{
			{
				ListenerName: "testListener1",
				ListenerPort: 0, // this one should get ephemeral port after PostUnmarshalTask
			},
			{
				ListenerName: "testListener2",
				ListenerPort: utilizedPort3, // this one should NOT get ephemeral port after PostUnmarshalTask
			},
			{
				ListenerName: "testListener3",
				ListenerPort: 0, // this one should get ephemeral port after PostUnmarshalTask
			},
		},
		EgressConfig: &serviceconnect.EgressConfig{
			ListenerName: "testEgressListener",
			ListenerPort: 0, // Presently this should always get ephemeral port
		},
	}
	originalSCConfig := cloneSCConfig(testSCConfig)
	task.ServiceConnectConfig = &testSCConfig
	assert.Nil(t, err, "Should be able to handle acs task")
	err = task.PostUnmarshalTask(&config.Config{}, nil, nil, nil, nil)
	assert.NoError(t, err)
	task.NetworkMode = AWSVPCNetworkMode

	validateServiceConnectContainerOrder(t, task)
	validateEphemeralPorts(t, task, originalSCConfig, utilizedPorts)
	validateAppnetEnvVars(t, task)

}

// TestPostUnmarshalTaskWithServiceConnectBridgeMode verifies pause container creation and container dependency/ordering
// for an SC-enabled bridge mode task. We verify:
// - regular taskContainer.CREATED depends on SCContainer.RESOURCES_PROVISIONED
// - regular taskContainer.CREATED depends on SCContainer.HEALTHY
// - a pause container is created for each regular task container, and has steady state RUNNING
// - SCContainer.PULLED depends on ALL pauseContainer.RUNNING
// - SCContainer.STOPPED depends on ALL taskContainer.STOPPED
// - pauseContainer.STOPPED depends on SCContainer.STOPPED
func TestPostUnmarshalTaskWithServiceConnectBridgeMode(t *testing.T) {
	const (
		utilizedPort1 = 33333
		utilizedPort2 = 44444
		listenerPort1 = 15000
		listenerPort2 = 16000
		listenerPort3 = 17000
	)
	utilizedPorts := map[uint16]struct{}{
		utilizedPort1: {},
		utilizedPort2: {},
		listenerPort1: {},
		listenerPort2: {},
		listenerPort3: {},
	}
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			containerFromACS("C1", utilizedPort1, 0, BridgeNetworkMode),
			containerFromACS("C2", utilizedPort2, 0, BridgeNetworkMode),
			containerFromACS(serviceConnectContainerTestName, 0, 0, BridgeNetworkMode),
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	testSCConfig := serviceconnect.Config{
		ContainerName: serviceConnectContainerTestName,
		IngressConfig: []serviceconnect.IngressConfigEntry{
			{
				ListenerName: "testListener1",
				ListenerPort: listenerPort1,
			},
			{
				ListenerName: "testListener2",
				ListenerPort: listenerPort2,
			},
			{
				ListenerName: "testListener3",
				ListenerPort: listenerPort3,
			},
		},
		EgressConfig: &serviceconnect.EgressConfig{
			ListenerName: "testEgressListener",
			ListenerPort: 0, // Presently this should always get ephemeral port
		},
	}
	originalSCConfig := cloneSCConfig(testSCConfig)
	task.ServiceConnectConfig = &testSCConfig
	assert.Nil(t, err, "Should be able to handle acs task")
	err = task.PostUnmarshalTask(&config.Config{}, nil, nil, nil, nil)
	assert.NoError(t, err)
	validateServiceConnectContainerOrder(t, task)
	validateEphemeralPorts(t, task, originalSCConfig, utilizedPorts)
	validateAppnetEnvVars(t, task)
	validateServiceConnectBridgeModePauseContainer(t, task)
}

func containerFromACS(name string, containerPort int64, hostPort int64, networkMode string) *ecsacs.Container {
	var portMapping *ecsacs.PortMapping
	if containerPort != 0 || hostPort != 0 {
		portMapping = &ecsacs.PortMapping{}
		if containerPort != 0 {
			portMapping.ContainerPort = aws.Int64(containerPort)
		}
		if hostPort != 0 {
			portMapping.HostPort = aws.Int64(hostPort)
		}
	}

	container := &ecsacs.Container{
		Name: aws.String(name),
		DockerConfig: &ecsacs.DockerConfig{
			HostConfig: aws.String(fmt.Sprintf(
				`{"NetworkMode":"%s"}`, networkMode)),
		},
	}
	if portMapping != nil {
		container.PortMappings = []*ecsacs.PortMapping{
			portMapping,
		}
	}
	return container
}

func cloneSCConfig(scConfig serviceconnect.Config) serviceconnect.Config {
	clone := scConfig
	clone.IngressConfig = nil
	for _, ic := range scConfig.IngressConfig {
		clone.IngressConfig = append(clone.IngressConfig, ic)
	}
	return clone
}

func validateServiceConnectContainerOrder(t *testing.T, task *Task) {
	c1, _ := task.ContainerByName("C1")
	c2, _ := task.ContainerByName("C2")
	scC, _ := task.ContainerByName(serviceConnectContainerTestName)

	// Check that regular containers have a dependency on SC container becoming HEALTHY
	assert.NotEmpty(t, c1.DependsOnUnsafe)
	assert.Equal(t, serviceConnectContainerTestName, c1.DependsOnUnsafe[0].ContainerName)
	assert.Equal(t, ContainerOrderingHealthyCondition, c1.DependsOnUnsafe[0].Condition)

	assert.NotEmpty(t, c2.DependsOnUnsafe)
	assert.Equal(t, serviceConnectContainerTestName, c2.DependsOnUnsafe[0].ContainerName)
	assert.Equal(t, ContainerOrderingHealthyCondition, c2.DependsOnUnsafe[0].Condition)

	// Check that SC container has a stop dependency on regular containers
	assert.Empty(t, scC.DependsOnUnsafe)
	assert.NotEmpty(t, scC.TransitionDependenciesMap)
	assert.NotEmpty(t, scC.TransitionDependenciesMap[apicontainerstatus.ContainerStopped].ContainerDependencies)
	assert.Equal(t, apicontainer.ContainerDependency{
		ContainerName:   "C1",
		SatisfiedStatus: apicontainerstatus.ContainerStopped,
	}, scC.TransitionDependenciesMap[apicontainerstatus.ContainerStopped].ContainerDependencies[0])
	assert.Equal(t, apicontainer.ContainerDependency{
		ContainerName:   "C2",
		SatisfiedStatus: apicontainerstatus.ContainerStopped,
	}, scC.TransitionDependenciesMap[apicontainerstatus.ContainerStopped].ContainerDependencies[1])
}

func validateEphemeralPorts(t *testing.T, task *Task, originalSCConfig serviceconnect.Config, utilizedPorts map[uint16]struct{}) {
	for i, ic := range originalSCConfig.IngressConfig {
		if ic.ListenerPort == 0 {
			assignedPort := task.ServiceConnectConfig.IngressConfig[i].ListenerPort
			_, ok := utilizedPorts[assignedPort]
			assert.Falsef(t, ok, "An already-utilized port [%d] was assigned to ingress listener: %s", assignedPort, ic.ListenerName)
			assert.NotZerof(t, assignedPort,
				"Ephemeral port was not assigned for ingress listener: %s", ic.ListenerName)
			utilizedPorts[assignedPort] = struct{}{}
		} else {
			assert.Equalf(t, ic.ListenerPort, task.ServiceConnectConfig.IngressConfig[i].ListenerPort,
				"Ingress port incorrectly modified for listener: %s", ic.ListenerName)
		}
		assert.Equalf(t, ic.HostPort, task.ServiceConnectConfig.IngressConfig[i].HostPort,
			"Ingress host port incorrectly modified for listener: %s", ic.ListenerName)
		assert.Equalf(t, ic.InterceptPort, task.ServiceConnectConfig.IngressConfig[i].InterceptPort,
			"Ingress intercept port incorrectly modified for listener: %s", ic.ListenerName)
		assert.Equalf(t, ic.ListenerName, task.ServiceConnectConfig.IngressConfig[i].ListenerName,
			"Ingress listener name incorrectly modified for listener: %s", ic.ListenerName)
	}
	if originalSCConfig.EgressConfig != nil && originalSCConfig.EgressConfig.ListenerPort == 0 {
		assignedPort := task.ServiceConnectConfig.EgressConfig.ListenerPort
		_, ok := utilizedPorts[assignedPort]
		assert.Falsef(t, ok, "An already-utilized port [%d] was assigned to egress listener", assignedPort)
		assert.NotZero(t, assignedPort,
			"Ephemeral port was not assigned for egress listener")
		utilizedPorts[assignedPort] = struct{}{}
	} else {
		assert.Equal(t, originalSCConfig.EgressConfig.ListenerPort, task.ServiceConnectConfig.EgressConfig.ListenerPort,
			"Egress port incorrectly modified for egress listener")
	}
	assert.Equalf(t, originalSCConfig.EgressConfig.ListenerName, task.ServiceConnectConfig.EgressConfig.ListenerName,
		"Egress listener name incorrectly modified")
	assert.Equalf(t, originalSCConfig.EgressConfig.VIP, task.ServiceConnectConfig.EgressConfig.VIP,
		"Egress VIP incorrectly modified")
}

func validateAppnetEnvVars(t *testing.T, task *Task) {
	// Validate the env vars were injected to SC container
	for _, c := range task.Containers {
		if c.Name != serviceConnectContainerTestName {
			continue
		}
		portMappingStr := c.Environment["APPNET_LISTENER_PORT_MAPPING"]
		// TODO [SC]: this map probably needs to change to the real Appnet model when it's ready
		portMapping := make(map[string]int)
		err := json.Unmarshal([]byte(portMappingStr), &portMapping)
		assert.NoError(t, err, "Error parsing APPNET_LISTENER_PORT_MAPPING")
		if task.IsNetworkModeAWSVPC() { // ECS Agent only select ephemeral listener ports for default SC AWSVPC tasks
			listener1 := task.ServiceConnectConfig.IngressConfig[0]
			listener3 := task.ServiceConnectConfig.IngressConfig[2]
			assert.Equalf(t, int(listener1.ListenerPort), portMapping[listener1.ListenerName], "Listener-port mapping incorrectly configured for %s", listener1.ListenerName)
			assert.Equalf(t, int(listener3.ListenerPort), portMapping[listener3.ListenerName], "Listener-port mapping incorrectly configured for %s", listener3.ListenerName)
		}
		egressListener := task.ServiceConnectConfig.EgressConfig
		assert.Equalf(t, int(egressListener.ListenerPort), portMapping[egressListener.ListenerName], "Listener-port mapping incorrectly configured for %s", egressListener.ListenerName)
	}
}

func validateServiceConnectBridgeModePauseContainer(t *testing.T, task *Task) {
	scC, _ := task.ContainerByName(serviceConnectContainerTestName)
	scPauseC, ok := task.ContainerByName(fmt.Sprintf("%s-%s", NetworkPauseContainerName, serviceConnectContainerTestName))
	assert.True(t, ok)
	p1, ok := task.ContainerByName(fmt.Sprintf("%s-%s", NetworkPauseContainerName, "C1"))
	assert.True(t, ok)
	p2, ok := task.ContainerByName(fmt.Sprintf("%s-%s", NetworkPauseContainerName, "C2"))
	assert.True(t, ok)
	pauseContainers := [...]*apicontainer.Container{p1, p2, scPauseC}

	// verify that SCContainer.CREATED depends on ALL PauseContainer.RESOURCES_PROVISIONED, and dthat
	// ALL PauseContainer.STOPPED depends on SCContainer.STOPPED
	assert.NotEmpty(t, scC.TransitionDependenciesMap)
	assert.NotNil(t, scC.TransitionDependenciesMap[apicontainerstatus.ContainerCreated])
	containerDependencies := scC.TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ContainerDependencies
	assert.Equal(t, len(pauseContainers), len(containerDependencies))
	for i, pc := range pauseContainers {
		assert.Equal(t, pc.Name, containerDependencies[i].ContainerName)
		assert.Equal(t, apicontainerstatus.ContainerResourcesProvisioned, containerDependencies[i].SatisfiedStatus)

		assert.NotEmpty(t, pc.TransitionDependenciesMap)
		assert.NotNil(t, pc.TransitionDependenciesMap[apicontainerstatus.ContainerStopped])
		assert.NotEmpty(t, pc.TransitionDependenciesMap[apicontainerstatus.ContainerStopped].ContainerDependencies)
		assert.Equal(t, apicontainer.ContainerDependency{
			ContainerName:   serviceConnectContainerTestName,
			SatisfiedStatus: apicontainerstatus.ContainerStopped,
		}, pc.TransitionDependenciesMap[apicontainerstatus.ContainerStopped].ContainerDependencies[0])
	}

	// verify that taskPauseContainer.RESOURCES_PROVISIONED depends on SCPauseContainer.RUNNING
	assert.NotNil(t, p1.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned])
	assert.NotEmpty(t, p1.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned].ContainerDependencies)
	assert.Equal(t, apicontainer.ContainerDependency{
		ContainerName:   scPauseC.Name,
		SatisfiedStatus: apicontainerstatus.ContainerRunning,
	}, p1.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned].ContainerDependencies[0])

	assert.NotNil(t, p2.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned])
	assert.NotEmpty(t, p2.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned].ContainerDependencies)
	assert.Equal(t, apicontainer.ContainerDependency{
		ContainerName:   scPauseC.Name,
		SatisfiedStatus: apicontainerstatus.ContainerRunning,
	}, p2.TransitionDependenciesMap[apicontainerstatus.ContainerResourcesProvisioned].ContainerDependencies[0])
}

func TestTaskFromACS_InitNetworkMode(t *testing.T) {
	for _, tc := range []struct {
		inputNetworkMode        string
		expectedTaskNetworkMode string
	}{
		{
			inputNetworkMode:        AWSVPCNetworkMode,
			expectedTaskNetworkMode: AWSVPCNetworkMode,
		},
		{
			inputNetworkMode:        BridgeNetworkMode,
			expectedTaskNetworkMode: BridgeNetworkMode,
		},
		{
			inputNetworkMode:        "",
			expectedTaskNetworkMode: BridgeNetworkMode,
		},
		{
			inputNetworkMode:        HostNetworkMode,
			expectedTaskNetworkMode: HostNetworkMode,
		},
	} {
		taskFromACS := ecsacs.Task{
			Arn:           strptr("myArn"),
			DesiredStatus: strptr("RUNNING"),
			Family:        strptr("myFamily"),
			Version:       strptr("1"),
			NetworkMode:   aws.String(tc.inputNetworkMode),
			Containers: []*ecsacs.Container{
				{
					Name: aws.String("C1"),
				},
				{
					Name: aws.String("C2"),
				},
			},
		}
		seqNum := int64(42)
		task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
		assert.Nil(t, err, "Should be able to handle acs task")
		assert.Equal(t, tc.expectedTaskNetworkMode, task.NetworkMode)
		switch tc.inputNetworkMode {
		case AWSVPCNetworkMode:
			assert.True(t, task.IsNetworkModeAWSVPC())
			assert.False(t, task.IsNetworkModeBridge())
			assert.False(t, task.IsNetworkModeHost())
		case BridgeNetworkMode, "":
			assert.False(t, task.IsNetworkModeAWSVPC())
			assert.True(t, task.IsNetworkModeBridge())
			assert.False(t, task.IsNetworkModeHost())
		case HostNetworkMode:
			assert.False(t, task.IsNetworkModeAWSVPC())
			assert.False(t, task.IsNetworkModeBridge())
			assert.True(t, task.IsNetworkModeHost())
		}
	}
}

func TestGetBridgeModePauseContainerForTaskContainer(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	container, err := testTask.GetBridgeModePauseContainerForTaskContainer(testTask.Containers[0])
	assert.Nil(t, err)
	assert.NotNil(t, container)
	assert.Equal(t, testTask.Containers[1].Name, container.Name)
}

func TestGetBridgeModePauseContainerForTaskContainer_NotFound(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.Containers[1].Name = "invalid"
	_, err := testTask.GetBridgeModePauseContainerForTaskContainer(testTask.Containers[0])
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "could not find pause container"))
}

func TestGetBridgeModeTaskContainerForPauseContainer(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	// make the service container name include "-" to make sure we can still resolve service container from pause container name correctly
	serviceContainerName := "service-container-name"
	testTask.Containers[0].Name = serviceContainerName
	testTask.Containers[1].Name = fmt.Sprintf("%s-%s", NetworkPauseContainerName, serviceContainerName)

	container, err := testTask.getBridgeModeTaskContainerForPauseContainer(testTask.Containers[1])
	assert.Nil(t, err)
	assert.NotNil(t, container)
	assert.Equal(t, testTask.Containers[0].Name, container.Name)
}

func TestGetBridgeModeTaskContainerForPauseContainer_InvalidPauseContainerName(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.Containers[1].Name = "invalid"
	_, err := testTask.getBridgeModeTaskContainerForPauseContainer(testTask.Containers[1])
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "does not conform to ~internal~ecs~pause-$TASK_CONTAINER_NAME format"))
}

func TestGetBridgeModeTaskContainerForPauseContainer_NotFound(t *testing.T) {
	testTask := getTestTaskServiceConnectBridgeMode()
	testTask.Containers[0].Name = "anotherTaskContainer"
	_, err := testTask.getBridgeModeTaskContainerForPauseContainer(testTask.Containers[1])
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "could not find task container"))
}

func TestTaskServiceConnectAttachment(t *testing.T) {
	seqNum := int64(42)
	tt := []struct {
		testName                    string
		testElasticNetworkInterface *ecsacs.ElasticNetworkInterface
		testNetworkMode             string
		testSCConfigValue           string
		testExpectedSCConfig        *serviceconnect.Config
	}{
		{
			testName: "Bridge default case",
			testElasticNetworkInterface: &ecsacs.ElasticNetworkInterface{
				Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
					{
						Primary:        aws.Bool(true),
						PrivateAddress: aws.String(ipv4),
					},
				},
			},
			testNetworkMode:   BridgeNetworkMode,
			testSCConfigValue: "{\"egressConfig\":{\"listenerName\":\"testOutboundListener\",\"vip\":{\"ipv4Cidr\":\"127.255.0.0/16\",\"ipv6Cidr\":\"\"}},\"dnsConfig\":[{\"hostname\":\"testHostName\",\"address\":\"172.31.21.40\"}],\"ingressConfig\":[{\"listenerPort\":15000}]}",
			testExpectedSCConfig: &serviceconnect.Config{
				ContainerName: serviceConnectContainerTestName,
				IngressConfig: []serviceconnect.IngressConfigEntry{
					{
						ListenerPort: testBridgeDefaultListenerPort,
					},
				},
				EgressConfig: &serviceconnect.EgressConfig{
					ListenerName: testOutboundListenerName,
					VIP: serviceconnect.VIP{
						IPV4CIDR: testIPv4Cidr,
						IPV6CIDR: "",
					},
				},
				DNSConfig: []serviceconnect.DNSConfigEntry{
					{
						HostName: testHostName,
						Address:  testIPv4Address,
					},
				},
			},
		},
		{
			testName: "AWSVPC override case with IPv6 enabled",
			testElasticNetworkInterface: &ecsacs.ElasticNetworkInterface{
				Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
					{
						Address: aws.String("ipv6"),
					},
				},
			},
			testNetworkMode:   AWSVPCNetworkMode,
			testSCConfigValue: "{\"egressConfig\":{\"listenerName\":\"testOutboundListener\",\"vip\":{\"ipv4Cidr\":\"127.255.0.0/16\",\"ipv6Cidr\":\"2002::1234:abcd:ffff:c0a8:101/64\"}},\"dnsConfig\":[{\"hostname\":\"testHostName\",\"address\":\"abcd:dcba:1234:4321::\"}],\"ingressConfig\":[{\"listenerPort\":8080}]}",
			testExpectedSCConfig: &serviceconnect.Config{
				ContainerName: serviceConnectContainerTestName,
				IngressConfig: []serviceconnect.IngressConfigEntry{
					{
						ListenerPort: testListenerPort,
					},
				},
				EgressConfig: &serviceconnect.EgressConfig{
					ListenerName: testOutboundListenerName,
					VIP: serviceconnect.VIP{
						IPV4CIDR: testIPv4Cidr,
						IPV6CIDR: testIPv6Cidr,
					},
				},
				DNSConfig: []serviceconnect.DNSConfigEntry{
					{
						HostName: testHostName,
						Address:  testIPv6Address,
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			taskFromACS := ecsacs.Task{
				Arn:                      strptr("myArn"),
				DesiredStatus:            strptr("RUNNING"),
				Family:                   strptr("myFamily"),
				Version:                  strptr("1"),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{tc.testElasticNetworkInterface},
				Containers: []*ecsacs.Container{
					containerFromACS("C1", 33333, 0, tc.testNetworkMode),
					containerFromACS(serviceConnectContainerTestName, 0, 0, tc.testNetworkMode),
				},
				Attachments: []*ecsacs.Attachment{
					{
						AttachmentArn: strptr("attachmentArn"),
						AttachmentProperties: []*ecsacs.AttachmentProperty{
							{
								Name:  strptr(serviceconnect.GetServiceConnectConfigKey()),
								Value: strptr(tc.testSCConfigValue),
							},
							{
								Name:  strptr(serviceconnect.GetServiceConnectContainerNameKey()),
								Value: strptr(serviceConnectContainerTestName),
							},
						},
						AttachmentType: strptr(serviceConnectAttachmentType),
					},
				},
				NetworkMode: strptr(tc.testNetworkMode),
			}
			task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
			assert.Nil(t, err, "Should be able to handle acs task")
			assert.Equal(t, tc.testNetworkMode, task.NetworkMode)
			assert.Equal(t, tc.testExpectedSCConfig, task.ServiceConnectConfig)
		})
	}
}

func TestTaskWithoutServiceConnectAttachment(t *testing.T) {
	seqNum := int64(42)
	testElasticNetworkInterface := &ecsacs.ElasticNetworkInterface{
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String(ipv4),
			},
		},
	}
	taskFromACS := ecsacs.Task{
		Arn:                      strptr("myArn"),
		DesiredStatus:            strptr("RUNNING"),
		Family:                   strptr("myFamily"),
		Version:                  strptr("1"),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{testElasticNetworkInterface},
		Containers: []*ecsacs.Container{
			containerFromACS("C1", 33333, 0, BridgeNetworkMode),
		},
		NetworkMode: strptr(BridgeNetworkMode),
	}

	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, BridgeNetworkMode, task.NetworkMode)
	assert.Nil(t, task.ServiceConnectConfig, "Should be no service connect config")
}

func TestRequiresCredentialSpecResource(t *testing.T) {
	container1 := &apicontainer.Container{}
	task1 := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container1},
	}

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	container2 := &apicontainer.Container{}
	container2.DockerConfig.HostConfig = &hostConfig
	task2 := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container2},
	}

	testCases := []struct {
		name           string
		task           *Task
		expectedOutput bool
	}{
		{
			name:           "missing_credentialspec",
			task:           task1,
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec",
			task:           task2,
			expectedOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.task.requiresCredentialSpecResource())
		})
	}

}

func TestGetAllCredentialSpecRequirements(t *testing.T) {
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	container := &apicontainer.Container{Name: "webapp1"}
	container.DockerConfig.HostConfig = &hostConfig

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container},
	}

	credentialSpecContainerMap := task.getAllCredentialSpecRequirements()

	credentialspecFileLocation := "credentialspec:file://gmsa_gmsa-acct.json"
	expectedCredentialSpecContainerMap := map[string]string{credentialspecFileLocation: "webapp1"}

	assert.True(t, reflect.DeepEqual(expectedCredentialSpecContainerMap, credentialSpecContainerMap))
}

func TestGetAllCredentialSpecRequirementsWithMultipleContainersUsingSameSpec(t *testing.T) {
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c1 := &apicontainer.Container{Name: "webapp1"}
	c1.DockerConfig.HostConfig = &hostConfig

	c2 := &apicontainer.Container{Name: "webapp2"}
	c2.DockerConfig.HostConfig = &hostConfig

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{c1, c2},
	}

	credentialSpecContainerMap := task.getAllCredentialSpecRequirements()

	credentialspecFileLocation := "credentialspec:file://gmsa_gmsa-acct.json"
	expectedCredentialSpecContainerMap := map[string]string{credentialspecFileLocation: "webapp2"}

	assert.Equal(t, len(expectedCredentialSpecContainerMap), len(credentialSpecContainerMap))
	assert.True(t, reflect.DeepEqual(expectedCredentialSpecContainerMap, credentialSpecContainerMap))
}

func TestGetAllCredentialSpecRequirementsWithMultipleContainers(t *testing.T) {
	hostConfig1 := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct-1.json\"]}"
	hostConfig2 := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct-2.json\"]}"

	c1 := &apicontainer.Container{Name: "webapp1"}
	c1.DockerConfig.HostConfig = &hostConfig1

	c2 := &apicontainer.Container{Name: "webapp2"}
	c2.DockerConfig.HostConfig = &hostConfig1

	c3 := &apicontainer.Container{Name: "webapp3"}
	c3.DockerConfig.HostConfig = &hostConfig2

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{c1, c2, c3},
	}

	credentialSpecContainerMap := task.getAllCredentialSpecRequirements()

	credentialspec1 := "credentialspec:file://gmsa_gmsa-acct-1.json"
	credentialspec2 := "credentialspec:file://gmsa_gmsa-acct-2.json"

	expectedCredentialSpecContainerMap := map[string]string{credentialspec1: "webapp2", credentialspec2: "webapp3"}

	assert.True(t, reflect.DeepEqual(expectedCredentialSpecContainerMap, credentialSpecContainerMap))
}

func TestGetCredentialSpecResource(t *testing.T) {
	credentialspecResource := &credentialspec.CredentialSpecResource{}
	task := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
	}
	task.AddResource(credentialspec.ResourceName, credentialspecResource)

	credentialspecTaskResource, ok := task.GetCredentialSpecResource()
	assert.True(t, ok)
	assert.NotEmpty(t, credentialspecTaskResource)
}
