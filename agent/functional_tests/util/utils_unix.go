// +build !windows,functional

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

package util

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

const (
	defaultExecDriverPath       = "/var/run/docker/execdriver"
	logdir                      = "/log"
	datadir                     = "/data"
	ExecDriverDir               = "/var/lib/docker/execdriver"
	defaultCgroupPath           = "/cgroup"
	defaultCgroupPathAgentMount = "/sys/fs/cgroup"
	cacheDirectory              = "/var/cache/ecs"
	configDirectory             = "/etc/ecs"
	readOnly                    = ":ro"
	dockerEndpoint              = "/var/run"
)

var ECS *ecs.ECS
var Cluster string

func init() {
	var ecsconfig aws.Config
	if region := os.Getenv("AWS_REGION"); region != "" {
		ecsconfig.Region = &region
	}
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		ecsconfig.Region = &region
	}
	if ecsconfig.Region == nil {
		if iid, err := ec2.NewEC2MetadataClient(nil).InstanceIdentityDocument(); err == nil {
			ecsconfig.Region = &iid.Region
		}
	}
	if envEndpoint := os.Getenv("ECS_BACKEND_HOST"); envEndpoint != "" {
		ecsconfig.Endpoint = &envEndpoint
	}

	ECS = ecs.New(session.New(&ecsconfig))
	Cluster = "ecs-functional-tests"
	if envCluster := os.Getenv("ECS_CLUSTER"); envCluster != "" {
		Cluster = envCluster
	}
	ECS.CreateCluster(&ecs.CreateClusterInput{
		ClusterName: aws.String(Cluster),
	})
}

// RunAgent launches the agent and returns an object which may be used to reference it.
// It will wait until the agent is correctly registered before returning.
// 'version' may be a docker image (e.g. amazon/amazon-ecs-agent:v1.0.0) with
// tag that may be used to run the agent. It defaults to
// 'amazon/amazon-ecs-agent:make', the version created locally by running
// 'make'
func RunAgent(t *testing.T, options *AgentOptions) *TestAgent {
	ctx := context.TODO()
	agent := &TestAgent{t: t}
	agentImage := "amazon/amazon-ecs-agent:make"
	if envImage := os.Getenv("ECS_AGENT_IMAGE"); envImage != "" {
		agentImage = envImage
	}
	agent.Image = agentImage

	dockerClient, err := docker.NewClientWithOpts(docker.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	if err != nil {
		t.Fatal(err)
	}
	agent.DockerClient = dockerClient

	_, _, err = dockerClient.ImageInspectWithRaw(ctx, agentImage)
	if err != nil {
		_, err = dockerClient.ImagePull(ctx, agentImage, types.ImagePullOptions{})
		if err != nil {
			t.Fatal("Could not launch agent", err)
		}
	}

	agentTempDir := ""
	if options != nil && options.TempDirOverride != "" {
		agentTempDir = options.TempDirOverride
	} else {
		tmpdirOverride := os.Getenv("ECS_FTEST_TMP")

		dir, err := ioutil.TempDir(tmpdirOverride, "ecs_integ_testdata")
		if err != nil {
			t.Fatal("Could not create temp dir for test")
		}
		agentTempDir = dir
	}

	logdir := filepath.Join(agentTempDir, "log")
	datadir := filepath.Join(agentTempDir, "data")
	os.Mkdir(logdir, 0755)
	os.Mkdir(datadir, 0755)
	agent.TestDir = agentTempDir
	agent.Options = options
	if options == nil {
		agent.Options = &AgentOptions{}
	}
	t.Logf("Created directory %s to store test data in", agentTempDir)

	err = agent.StartAgent()
	if err != nil {
		t.Fatal(err)
	}

	if options != nil && options.EnableTaskENI {
		// if task networking is enabled, needs to wait for container instance to become active
		err = agent.WaitContainerInstanceStatus("ACTIVE", t)
		// If we get an error here, we need to stop the agent before failing, since the caller won't be stopping it.
		if err != nil {
			t.Logf("Failed to wait for container instance to reach ACTIVE: %v", err)
			t.Logf("Stopping agent container: %s", agent.DockerID)
			errS := agent.StopAgent()
			require.NoError(t, errS)
			t.Fatalf("Failed to wait for container instance to reach ACTIVE: %v", err)
		}
	}
	return agent
}

func (agent *TestAgent) StopAgent() error {
	ctx := context.TODO()
	containerStopTimeout := 10 * time.Second
	return agent.DockerClient.ContainerStop(ctx, agent.DockerID, &containerStopTimeout)
}

func (agent *TestAgent) StartAgent() error {
	agent.t.Logf("Launching agent with image: %s\n", agent.Image)
	dockerConfig := &dockercontainer.Config{
		Image: agent.Image,
		ExposedPorts: map[nat.Port]struct{}{
			"51678/tcp": {},
		},
		Env: []string{
			"ECS_CLUSTER=" + Cluster,
			"ECS_DATADIR=/data",
			"ECS_HOST_DATA_DIR=" + agent.TestDir,
			"ECS_LOGLEVEL=debug",
			"ECS_LOGFILE=/log/integ_agent.log",
			"ECS_BACKEND_HOST=" + os.Getenv("ECS_BACKEND_HOST"),
			"AWS_ACCESS_KEY_ID=" + os.Getenv("AWS_ACCESS_KEY_ID"),
			"AWS_DEFAULT_REGION=" + *ECS.Config.Region,
			"AWS_SECRET_ACCESS_KEY=" + os.Getenv("AWS_SECRET_ACCESS_KEY"),
			"ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION=" + os.Getenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION"),
		},
		Cmd: strings.Split(os.Getenv("ECS_FTEST_AGENT_ARGS"), " "),
	}

	// Append ECS_INSTANCE_ATTRIBUTES to dockerConfig
	if attr := os.Getenv("ECS_INSTANCE_ATTRIBUTES"); attr != "" {
		dockerConfig.Env = append(dockerConfig.Env, "ECS_INSTANCE_ATTRIBUTES="+attr)
	}

	binds := agent.getBindMounts()

	hostConfig := &dockercontainer.HostConfig{
		Binds: binds,
		PortBindings: map[nat.Port][]nat.PortBinding{
			"51678/tcp": {{HostIP: "0.0.0.0"}},
		},
		NetworkMode: "host",
	}

	if agent.Options != nil {
		// Override the default docker environment variable
		for key, value := range agent.Options.ExtraEnvironment {
			envVarExists := false
			for i, str := range dockerConfig.Env {
				if strings.HasPrefix(str, key+"=") {
					dockerConfig.Env[i] = key + "=" + value
					envVarExists = true
					break
				}
			}
			if !envVarExists {
				dockerConfig.Env = append(dockerConfig.Env, key+"="+value)
			}
		}

		for key, value := range agent.Options.PortBindings {
			hostConfig.PortBindings[key] = []nat.PortBinding{{HostIP: value["HostIP"], HostPort: value["HostPort"]}}
			dockerConfig.ExposedPorts[key] = struct{}{}
		}

		hostCofigInit := true
		if agent.Options.EnableTaskENI {
			dockerConfig.Env = append(dockerConfig.Env, "ECS_ENABLE_TASK_ENI=true")
			hostConfig.Binds = append(hostConfig.Binds,
				"/lib64:/lib64:ro",
				"/proc:/host/proc:ro",
				"/var/lib/ecs/dhclient:/var/lib/ecs/dhclient",
				"/sbin:/host/sbin:ro",
				"/lib:/lib:ro",
				"/usr/lib:/usr/lib:ro",
				"/usr/lib64:/usr/lib64:ro",
			)

			hostConfig.CapAdd = []string{"NET_ADMIN", "SYS_ADMIN"}
			hostConfig.Init = &hostCofigInit
			hostConfig.NetworkMode = "host"
		}

	}

	ctx := context.TODO()
	agentContainer, err := agent.DockerClient.ContainerCreate(ctx,
		dockerConfig,
		hostConfig,
		&network.NetworkingConfig{},
		"")
	if err != nil {
		agent.t.Fatal("Could not create agent container", err)
	}
	agent.DockerID = agentContainer.ID
	agent.t.Logf("Agent started as docker container: %s\n", agentContainer.ID)

	err = agent.DockerClient.ContainerStart(ctx, agentContainer.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.New("Could not start agent container " + err.Error())
	}

	containerJSON, err := agent.DockerClient.ContainerInspect(ctx, agentContainer.ID)
	if err != nil {
		return errors.New("Could not inspect agent container: " + err.Error())
	}
	if containerJSON.HostConfig.NetworkMode == "host" {
		agent.IntrospectionURL = "http://localhost:51678"
	} else {
		agent.IntrospectionURL = "http://localhost:" + containerJSON.NetworkSettings.Ports["51678/tcp"][0].HostPort
	}

	return agent.verifyIntrospectionAPI()
}

// getBindMounts actually constructs volume binds for container's host config
// It also additionally checks for environment variables:
// * CGROUP_PATH: the cgroup path
// * EXECDRIVER_PATH: the path of metrics
func (agent *TestAgent) getBindMounts() []string {
	var binds []string
	cgroupPath := utils.DefaultIfBlank(os.Getenv("CGROUP_PATH"), defaultCgroupPath)
	cgroupBind := cgroupPath + ":" + defaultCgroupPathAgentMount
	binds = append(binds, cgroupBind)

	execdriverPath := utils.DefaultIfBlank(os.Getenv("EXECDRIVER_PATH"), defaultExecDriverPath)
	execdriverBind := execdriverPath + ":" + ExecDriverDir + readOnly
	binds = append(binds, execdriverBind)

	hostLogDir := filepath.Join(agent.TestDir, "log")
	hostDataDir := filepath.Join(agent.TestDir, "data")
	hostConfigDir := filepath.Join(agent.TestDir, "config")
	hostCacheDir := filepath.Join(agent.TestDir, "cache")
	agent.Logdir = hostLogDir

	binds = append(binds, hostLogDir+":"+logdir)
	binds = append(binds, hostDataDir+":"+datadir)
	binds = append(binds, dockerEndpoint+":"+dockerEndpoint)
	binds = append(binds, hostConfigDir+":"+configDirectory)
	binds = append(binds, hostCacheDir+":"+cacheDirectory)

	if agent.Options != nil {
		if agent.Options.GPUEnabled {
			// bind mount the GPU info directory on the instance created by init
			binds = append(binds, gpu.GPUInfoDirPath+":"+gpu.GPUInfoDirPath)
		}
	}

	return binds
}

func (agent *TestAgent) Cleanup() {
	if agent.Options == nil || !agent.Options.EnableTaskENI {
		// if task networking is not enabled, do the usual cleanup
		agent.platformIndependentCleanup()
		return
	}

	// otherwise, we need to wait for container instance to become INACTIVE after deregistration.
	// cleanup needs to happen regardless of what happened elsewhere.
	defer func() {
		if agent.t.Failed() {
			agent.t.Logf("Preserving test dir for failed test %s", agent.TestDir)
		} else {
			agent.t.Logf("Removing test dir for passed test %s", agent.TestDir)
			os.RemoveAll(agent.TestDir)
		}
	}()

	// stop the agent
	err := agent.StopAgent()
	require.NoError(agent.t, err)

	// deregister the container instance
	_, err = ECS.DeregisterContainerInstance(&ecs.DeregisterContainerInstanceInput{
		Cluster:           &agent.Cluster,
		ContainerInstance: &agent.ContainerInstanceArn,
		Force:             aws.Bool(true),
	})
	require.NoError(agent.t, err)

	// wait for container instance to reach INACTIVE
	err = agent.WaitContainerInstanceStatus("INACTIVE", agent.t)
	require.NoError(agent.t, err)
}
