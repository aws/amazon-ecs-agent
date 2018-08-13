// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/cihub/seelog"
)

const (
	agentIntrospectionEndpoint = "http://localhost:51678/v1/"
	maxRetries                 = 4
	durationBetweenRetries     = time.Second
	containerName              = "agent-introspection-validator"
	tasksMetadataRespType      = "tasks metadata"
	taskMetadataRespType       = "task metadata"
)

// TaskResponse is the schema for the task response JSON object
type TaskResponse struct {
	Arn           string              `json:"Arn"`
	DesiredStatus string              `json:"DesiredStatus,omitempty"`
	KnownStatus   string              `json:"KnownStatus"`
	Family        string              `json:"Family"`
	Version       string              `json:"Version"`
	Containers    []ContainerResponse `json:"Containers"`
}

// TasksResponse is the schema for the tasks response JSON object
type TasksResponse struct {
	Tasks []*TaskResponse `json:"Tasks"`
}

// ContainerResponse is the schema for the container response JSON object
type ContainerResponse struct {
	DockerID   string         `json:"DockerId"`
	DockerName string         `json:"DockerName"`
	Name       string         `json:"Name"`
	Ports      []PortResponse `json:"Ports,omitempty"`
	Networks   Network        `json:"Networks,omitempty"`
}

// PortResponse defines the schema for portmapping response JSON
// object.
type PortResponse struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
}

// Network is a struct that keeps track of metadata of a network interface
type Network struct {
	NetworkMode   string   `json:"NetworkMode,omitempty"`
	IPv4Addresses []string `json:"IPv4Addresses,omitempty"`
	IPv6Addresses []string `json:"IPv6Addresses,omitempty"`
}

func getTasksMetadata(client *http.Client, path string) (*TasksResponse, error) {
	body, err := metadataResponse(client, path, tasksMetadataRespType)
	if err != nil {
		return nil, err
	}

	seelog.Infof("Received tasks metadata: %s \n", string(body))

	err = verifyTasksMetadata(body)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to verify response: %v", tasksMetadataRespType, err)
	}

	var tasksMetadata TasksResponse
	err = json.Unmarshal(body, &tasksMetadata)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to parse response body: %v", tasksMetadataRespType, err)
	}

	return &tasksMetadata, nil
}

func getTaskMetadata(client *http.Client, path string) (*TaskResponse, error) {
	body, err := metadataResponse(client, path, taskMetadataRespType)
	if err != nil {
		return nil, err
	}

	seelog.Infof("Received task metadata: %s \n", string(body))

	err = verifyTaskMetadata(body)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to verify response: %v", taskMetadataRespType, err)
	}

	var taskMetadata TaskResponse
	err = json.Unmarshal(body, &taskMetadata)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to parse response body: %v", taskMetadataRespType, err)
	}

	return &taskMetadata, nil
}

// verifyTasksMetadata verifies the number of tasks in tasks metadata.
func verifyTasksMetadata(tasksMetadataRawMsg json.RawMessage) error {
	var tasksMetadataMap map[string]json.RawMessage
	json.Unmarshal(tasksMetadataRawMsg, &tasksMetadataMap)

	if tasksMetadataMap["Tasks"] == nil {
		return notEmptyErrMsg("Tasks")
	}

	var tasksMetadataArray []json.RawMessage
	json.Unmarshal(tasksMetadataMap["Tasks"], &tasksMetadataArray)

	if len(tasksMetadataArray) != 1 {
		return fmt.Errorf("incorrect number of tasks, expected 1, received %d",
			len(tasksMetadataArray))
	}

	return verifyTaskMetadata(tasksMetadataArray[0])
}

// verifyTaskMetadata verifies the number of containers in task metadata, make
// sure each necessary field is not empty, we cannot check the values of those fields
// since they are dynamic. It also verifies the container metadata.
func verifyTaskMetadata(taskMetadataRawMsg json.RawMessage) error {
	taskMetadataMap := make(map[string]json.RawMessage)
	json.Unmarshal(taskMetadataRawMsg, &taskMetadataMap)

	if taskMetadataMap["Arn"] == nil {
		return notEmptyErrMsg("Arn")
	}

	if taskMetadataMap["DesiredStatus"] == nil {
		return notEmptyErrMsg("DesiredStatus")
	}

	if taskMetadataMap["KnownStatus"] == nil {
		return notEmptyErrMsg("KnownStatus")
	}

	if taskMetadataMap["Family"] == nil {
		return notEmptyErrMsg("Family")
	}

	if taskMetadataMap["Version"] == nil {
		return notEmptyErrMsg("Version")
	}

	if taskMetadataMap["Containers"] == nil {
		return notEmptyErrMsg("Containers")
	}

	var containersMetadataArray []json.RawMessage
	json.Unmarshal(taskMetadataMap["Containers"], &containersMetadataArray)
	if len(containersMetadataArray) != 1 {
		return fmt.Errorf("incorrect number of containers, expected 1, received %d",
			len(containersMetadataArray))
	}

	return verifyContainerMetadata(containersMetadataArray[0])
}

// verifyContainerMetadata verifies the container name of the container metadata,
// and each necessary field is not empty.
func verifyContainerMetadata(containerMetadataRawMsg json.RawMessage) error {
	containerMetadataMap := make(map[string]json.RawMessage)
	json.Unmarshal(containerMetadataRawMsg, &containerMetadataMap)

	if containerMetadataMap["Name"] == nil {
		return notEmptyErrMsg("Name")
	}

	var actualContainerName string
	json.Unmarshal(containerMetadataMap["Name"] ,&actualContainerName)

	if actualContainerName != containerName {
		return fmt.Errorf("incorrect container name, expected %s, received %s",
			containerName, actualContainerName)
	}

	if containerMetadataMap["DockerId"] == nil {
		return notEmptyErrMsg("DockerId")
	}

	if containerMetadataMap["DockerName"] == nil {
		return notEmptyErrMsg("DockerName")
	}

	return nil
}

func notEmptyErrMsg(fieldName string) error {
	return fmt.Errorf("field %s should not be empty", fieldName)
}

func metadataResponse(client *http.Client, endpoint string, respType string) ([]byte, error) {
	var resp []byte
	var err error
	for i := 0; i < maxRetries; i++ {
		resp, err = metadataResponseOnce(client, endpoint, respType)
		if err == nil {
			return resp, nil
		}
		fmt.Fprintf(os.Stderr, "Attempt [%d/%d]: unable to get metadata response for '%s' from '%s': %v",
			i, maxRetries, respType, endpoint, err)
		time.Sleep(durationBetweenRetries)
	}

	return nil, err
}

func metadataResponseOnce(client *http.Client, endpoint string, respType string) ([]byte, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to get response: %v", respType, err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: incorrect status code  %d", respType, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: unable to read response body: %v", respType, err)
	}

	return body, nil
}

func getTaskARNFromTasksMetadata(tasksMetadata *TasksResponse) string {
	return tasksMetadata.Tasks[0].Arn
}

func getDockerIDFromTaskMetadata(taskMetadata *TaskResponse) string {
	return taskMetadata.Containers[0].DockerID
}

func main() {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	tasksMetadataPath := agentIntrospectionEndpoint + "tasks"
	tasksMetadata, err := getTasksMetadata(client, tasksMetadataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get tasks metadata for '%s': %v", tasksMetadataPath, err)
		os.Exit(1)
	}

	// Use task arn to get task metadata
	taskARN := getTaskARNFromTasksMetadata(tasksMetadata)
	taskMetadataTaskARNPath := agentIntrospectionEndpoint + "tasks?taskarn=" + taskARN
	taskMetadata, err := getTaskMetadata(client, taskMetadataTaskARNPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get task metadata for '%s': %v", taskMetadataTaskARNPath, err)
		os.Exit(1)
	}

	// Use docker id to get task metadata
	dockerID := getDockerIDFromTaskMetadata(taskMetadata)
	taskMetadataDockerIDPath := agentIntrospectionEndpoint + "tasks?dockerid=" + dockerID
	_, err = getTaskMetadata(client, taskMetadataDockerIDPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get task metadata for '%s': %v", taskMetadataDockerIDPath, err)
		os.Exit(1)
	}

	os.Exit(42)
}
