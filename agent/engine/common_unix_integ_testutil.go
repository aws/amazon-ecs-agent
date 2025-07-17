//go:build linux && (sudo || integration)
// +build linux
// +build sudo integration

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

package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRegistryImage           = "127.0.0.1:51670/amazon/amazon-ecs-netkitten:latest"
	dockerEndpoint              = "unix:///var/run/docker.sock"
	validTaskArnPrefix          = "arn:aws:ecs:region:account-id:task/"
	testAppContainerImage       = "public.ecr.aws/docker/library/busybox:latest"
	TestCluster                 = "testCluster"
	TestInstanceID              = "testInstanceID"
	firelensCheckLogMaxAttempts = 60
)

var (
	mockTaskMetadataHandler = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(`{"TaskARN": "arn:aws:ecs:region:account-id:task/task-id"}`))
	})
)

func CreateTestContainer() *apicontainer.Container {
	return createTestContainerWithImageAndName(testRegistryImage, "netcat")
}

// GetLongRunningCommand returns the command that keeps the container running for the container
// that uses the default integ test image (amazon/amazon-ecs-netkitten for unix)
func GetLongRunningCommand() []string {
	return []string{"-loop=true"}
}

func isDockerRunning() bool {
	_, err := os.Stat("/var/run/docker.sock")
	return err == nil
}

// createFirelensLogDir creates a temporary directory where the log router container can write logs to
func createFirelensLogDir(t *testing.T, directoryPrefix, firelensContainerUser string) string {
	// os.MkdirTemp creates a dir with a random string appended to its name.
	// Reference: https://pkg.go.dev/os#MkdirTemp
	tmpDir, err := os.MkdirTemp("", directoryPrefix)
	require.NoError(t, err)

	// Change log directory ownership so that the log router can write logs to it,
	// especially when running as non-root user
	if firelensContainerUser != "" {
		// Parse the user string to get UID and GID
		parts := strings.Split(firelensContainerUser, ":")
		if len(parts) == 2 {
			uid, err := strconv.Atoi(parts[0])
			require.NoError(t, err)

			gid, err := strconv.Atoi(parts[1])
			require.NoError(t, err)

			// Change ownership of the directory
			err = os.Chown(tmpDir, uid, gid)
			require.NoError(t, err)
		}
	}
	return tmpDir
}

// setupTaskMetadataServer sets up a mock TMDS that the Firelens container needs access to.
func setupTaskMetadataServer(t *testing.T) {
	// Note that the listener has to be overwritten here, because the default one from httptest.NewServer
	// only listens to localhost and isn't reachable from container running in bridge network mode.
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(mockTaskMetadataHandler)
	server.Listener = l
	server.Start()
	defer server.Close()
	port := getPortFromAddr(t, server.URL)
	ec2MetadataClient, err := ec2.NewEC2MetadataClient(nil)
	require.NoError(t, err)
	serverURL := fmt.Sprintf("http://%s:%s", getHostPrivateIP(t, ec2MetadataClient), port)
	defer setV3MetadataURLFormat(serverURL + "/v3/%s")()
}

// getPortFromAddr returns the port part of an address in format "http://<addr>:<port>".
func getPortFromAddr(t *testing.T, addr string) string {
	u, err := url.Parse(addr)
	require.NoErrorf(t, err, "unable to parse address: %s", addr)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoErrorf(t, err, "unable to get port from address: %s", addr)
	return port
}

// getHostPrivateIP returns the host's private IP.
func getHostPrivateIP(t *testing.T, ec2MetadataClient ec2.EC2MetadataClient) string {
	ip, err := ec2MetadataClient.PrivateIPv4Address()
	require.NoError(t, err)
	return ip
}

// setV3MetadataURLFormat sets the container metadata URI format and returns a function to set it back.
func setV3MetadataURLFormat(fmt string) func() {
	backup := apicontainer.MetadataURIFormat
	apicontainer.MetadataURIFormat = fmt
	return func() {
		apicontainer.MetadataURIFormat = backup
	}
}

// verifyFirelensTaskEvents verifies the Firelens task and container events based on the execution phase.
func verifyFirelensTaskEvents(t *testing.T, taskEngine TaskEngine, firelensTask *apitask.Task, phase string) {
	testEvents := InitTestEventCollection(taskEngine)
	switch phase {
	case "start":
		// Verify the log-router container is running
		VerifyContainerStatus(apicontainerstatus.ContainerRunning, firelensTask.Arn+":log-router", testEvents, t)

		// Verify the app container is running
		VerifyContainerStatus(apicontainerstatus.ContainerRunning, firelensTask.Arn+":app", testEvents, t)

		// Verify the task is in running state
		VerifyTaskStatus(apitaskstatus.TaskRunning, firelensTask.Arn, testEvents, t)
	case "stop":
		// Verify the app container is stopped
		VerifyContainerStatus(apicontainerstatus.ContainerStopped, firelensTask.Arn+":app", testEvents, t)

		// Verify the log-router container is stopped
		VerifyContainerStatus(apicontainerstatus.ContainerStopped, firelensTask.Arn+":log-router", testEvents, t)

		// Verify the task has stopped
		VerifyTaskStatus(apitaskstatus.TaskStopped, firelensTask.Arn, testEvents, t)
	default:
		t.Errorf("Unexpected phase: %s", phase)
	}
}

// createFirelensTask creates an ECS task with 2 containers – 1) App 2) Log router container
// The app writes the date to stdout, and the log router forwards it to a local file on disk
func createFirelensTask(t *testing.T, testName, firelensContainerImage, firelensContainerUser,
	firelensType, tmpDir string) *apitask.Task {
	// Generate the config depending on which log router is being used.
	var rawHostConfigInputForApp dockercontainer.HostConfig
	if firelensType == firelens.FirelensConfigTypeFluentbit {
		rawHostConfigInputForApp = dockercontainer.HostConfig{
			LogConfig: dockercontainer.LogConfig{
				Type: logDriverTypeFirelens,
				Config: map[string]string{
					"Name":   "file",
					"Path":   "/logs",
					"File":   "app.log",
					"Format": "plain",
				},
			},
		}
	} else {
		rawHostConfigInputForApp = dockercontainer.HostConfig{
			LogConfig: dockercontainer.LogConfig{
				Type: logDriverTypeFirelens,
				Config: map[string]string{
					"@type":  "file",
					"path":   "/logs/",
					"format": "json",
					"append": "true",
				},
			},
		}
	}
	rawHostConfigForApp, err := json.Marshal(&rawHostConfigInputForApp)
	require.NoError(t, err)

	testTask := CreateTestTask(validTaskArnPrefix + testName + "-" + uuid.New())
	testTask.Containers = []*apicontainer.Container{
		{
			Name:      "app",
			Image:     testAppContainerImage,
			Essential: true,
			// Since the app container is essential, the app exit also immediately makes the log router container stop.
			// Sleeping here before the exit gives the log router sometime to process the log and write to the destination.
			Command: []string{"sh", "-c",
				"date +%T; sleep 10; exit 42"},
			DockerConfig: apicontainer.DockerConfig{
				HostConfig: func() *string {
					s := string(rawHostConfigForApp)
					return &s
				}(),
			},
		},
		{
			Name:      "log-router",
			Image:     firelensContainerImage,
			Essential: true,
			FirelensConfig: &apicontainer.FirelensConfig{
				Type: firelensType,
				Options: map[string]string{
					"enable-ecs-log-metadata": "true",
				},
			},
			Environment: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_ECS_EC2",
				"FLB_LOG_LEVEL":     "debug",
			},
			MountPoints: []apicontainer.MountPoint{
				{
					SourceVolume:  "logs",
					ContainerPath: "/logs",
				},
			},
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
		},
	}

	// Set firelens container user when defined
	if firelensContainerUser != "" {
		rawConfigForLogRouter := fmt.Sprintf(`{"User":"%s"}`, firelensContainerUser)
		testTask.Containers[1].DockerConfig = apicontainer.DockerConfig{
			Config: func() *string {
				return &rawConfigForLogRouter
			}(),
		}
	}

	// Initialize a task volume where the app logs will be written to
	testTask.Volumes = []apitask.TaskVolume{
		{
			Name:   "logs",
			Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: tmpDir},
		},
	}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	return testTask
}

// verifyFirelensDataDir verifies the configuration of the data directory, that is created for the Firelens task
func verifyFirelensDataDir(t *testing.T, firelensDataDir, firelensContainerUser string) {
	// Verify Firelens data directory exists
	_, err := os.Stat(firelensDataDir)
	require.NoError(t, err)

	// Validate subdirectories - config and socket
	firelensConfigDir := filepath.Join(firelensDataDir, "config")
	firelensSocketDir := filepath.Join(firelensDataDir, "socket")

	// Verify config directory exists
	configDirInfo, err := os.Stat(firelensConfigDir)
	require.NoError(t, err)

	// Verify socket directory exists
	socketDir, err := os.Stat(firelensSocketDir)
	require.NoError(t, err)

	// Helper function to get UID:GID string from FileInfo
	getOwnership := func(info os.FileInfo) string {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatalf("Failed to get stat info")
			return ""
		}
		return fmt.Sprintf("%d:%d", stat.Uid, stat.Gid)
	}

	// Verify the config directory is owned by the ECS agent (root)
	expectedAgentOwnership := fmt.Sprintf("%d:%d", 0, 0)
	assert.Equal(t, expectedAgentOwnership, getOwnership(configDirInfo),
		"Firelens config directory has incorrect ownership")

	// Verify the socket directory is owned by the firelensContainerUser when specified
	if firelensContainerUser == "" {
		assert.Equal(t, expectedAgentOwnership, getOwnership(configDirInfo),
			"Firelens config directory has incorrect ownership")
	} else {
		assert.Equal(t, firelensContainerUser, getOwnership(socketDir),
			"Firelens socket directory has incorrect ownership")
	}
}

// verifyFirelensLogs verifies that the Firelens container sent app logs to a local file as expected
func verifyFirelensLogs(t *testing.T, firelensTask *apitask.Task, firelensType, tmpDir string) {
	// Define the expected log file path based on the firelens type
	var logFilePath string
	var logFilePattern string
	if firelensType == firelens.FirelensConfigTypeFluentbit {
		// For Fluentbit, we expect a specific file name
		logFilePath = filepath.Join(tmpDir, "app.log")
		logFilePattern = logFilePath // Exact file path
	} else {
		// For Fluentd, we'll look for any .log file, since the file name is not deterministic
		logFilePattern = filepath.Join(tmpDir, "*.log")
	}

	// Wait for the log file to appear and have content
	var logContent []byte
	var err error
	var found bool
	for i := 0; i < firelensCheckLogMaxAttempts; i++ {
		if firelensType == firelens.FirelensConfigTypeFluentbit {
			// For Fluentbit, check the specific file
			logContent, err = os.ReadFile(logFilePath)
			found = err == nil && len(logContent) > 0
		} else {
			// For Fluentd, find any .log file
			matches, globErr := filepath.Glob(logFilePattern)
			if globErr == nil && len(matches) > 0 {
				// Use the first match
				logFilePath = matches[0]
				logContent, err = os.ReadFile(logFilePath)
				found = err == nil && len(logContent) > 0
			}
		}
		if found {
			t.Logf("Found log file with content at %s", logFilePath)
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Verify we found a log file with content
	require.True(t, found, "Failed to find app logs")

	// Parse the JSON log object
	jsonBlob := make(map[string]string)
	err = json.Unmarshal(logContent, &jsonBlob)
	require.NoError(t, err, "Failed to parse JSON log entry")

	// Verify log fields
	assert.Contains(t, jsonBlob, "container_id", "Log missing container_id field")
	assert.Contains(t, jsonBlob, "container_name", "Log missing container_name field")
	assert.Contains(t, jsonBlob["container_name"], "app", "Log from wrong container")
	assert.Equal(t, "stdout", jsonBlob["source"], "Incorrect log source")

	// Verify ECS metadata fields
	assert.Equal(t, TestInstanceID, jsonBlob["ec2_instance_id"], "Incorrect EC2 instance ID")
	assert.Equal(t, TestCluster, jsonBlob["ecs_cluster"], "Incorrect cluster name")
	assert.Equal(t, firelensTask.Arn, jsonBlob["ecs_task_arn"], "Incorrect task ARN")
	assert.Contains(t, jsonBlob, "ecs_task_definition", "Task definition missing")
}

// verifyFirelensResourceCleanup verifies that the Firelens task resource is cleaned up once the task stops
func verifyFirelensResourceCleanup(t *testing.T, cfg *config.Config, firelensTask *apitask.Task, taskEngine TaskEngine,
	firelensDataDir string) {
	// Set sent status to trigger task cleanup
	t.Logf("SentStatus for task %s has been marked as stopped", firelensTask.Arn)
	firelensTask.SetSentStatus(apitaskstatus.TaskStopped)
	waitForTaskCleanup(t, taskEngine, firelensTask.Arn, 30)
	// Make sure the data directory is cleaned up
	_, err := os.ReadDir(firelensDataDir)
	assert.True(t, os.IsNotExist(err), "Firelens data directory has not been cleaned up")
}
