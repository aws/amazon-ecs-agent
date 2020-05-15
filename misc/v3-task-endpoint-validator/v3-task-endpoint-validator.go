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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const (
	containerMetadataEnvVar = "ECS_CONTAINER_METADATA_URI"
	maxRetries              = 4
	durationBetweenRetries  = time.Second
)

var isAWSVPCNetworkMode bool
var isBridgeNetworkMode bool
var checkContainerInstanceTags bool
var networkModes map[string]bool

// TaskResponse defines the schema for the task response JSON object
type TaskResponse struct {
	Cluster            string              `json:"Cluster"`
	TaskARN            string              `json:"TaskARN"`
	Family             string              `json:"Family"`
	Revision           string              `json:"Revision"`
	DesiredStatus      string              `json:"DesiredStatus,omitempty"`
	KnownStatus        string              `json:"KnownStatus"`
	AvailabilityZone   string              `json:"AvailabilityZone"`
	Containers         []ContainerResponse `json:"Containers,omitempty"`
	Limits             *LimitsResponse     `json:"Limits,omitempty"`
	PullStartedAt      *time.Time          `json:"PullStartedAt,omitempty"`
	PullStoppedAt      *time.Time          `json:"PullStoppedAt,omitempty"`
	ExecutionStoppedAt *time.Time          `json:"ExecutionStoppedAt,omitempty"`
}

// ContainerResponse defines the schema for the container response
// JSON object
type ContainerResponse struct {
	ID            string            `json:"DockerId"`
	Name          string            `json:"Name"`
	DockerName    string            `json:"DockerName"`
	Image         string            `json:"Image"`
	ImageID       string            `json:"ImageID"`
	Ports         []PortResponse    `json:"Ports,omitempty"`
	Labels        map[string]string `json:"Labels,omitempty"`
	DesiredStatus string            `json:"DesiredStatus"`
	KnownStatus   string            `json:"KnownStatus"`
	ExitCode      *int              `json:"ExitCode,omitempty"`
	Limits        LimitsResponse    `json:"Limits"`
	CreatedAt     *time.Time        `json:"CreatedAt,omitempty"`
	StartedAt     *time.Time        `json:"StartedAt,omitempty"`
	FinishedAt    *time.Time        `json:"FinishedAt,omitempty"`
	Type          string            `json:"Type"`
	Networks      []Network         `json:"Networks,omitempty"`
	Health        HealthStatus      `json:"Health,omitempty"`
}

// LimitsResponse defines the schema for task/cpu limits response
// JSON object
type LimitsResponse struct {
	CPU    *float64 `json:"CPU,omitempty"`
	Memory *int64   `json:"Memory,omitempty"`
}

type HealthStatus struct {
	Status   string     `json:"status,omitempty"`
	Since    *time.Time `json:"statusSince,omitempty"`
	ExitCode int        `json:"exitCode,omitempty"`
	Output   string     `json:"output,omitempty"`
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

func verifyContainerMetadata(client *http.Client, containerMetadataEndpoint string) error {
	var err error
	body, err := metadataResponse(client, containerMetadataEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received container metadata: %s \n", string(body))

	var containerMetadata ContainerResponse
	if err = json.Unmarshal(body, &containerMetadata); err != nil {
		return fmt.Errorf("unable to parse response body: %v", err)
	}

	if err = verifyContainerMetadataResponse(body); err != nil {
		return err
	}

	return nil
}

func verifyTaskMetadata(client *http.Client, taskMetadataEndpoint string) error {
	body, err := metadataResponse(client, taskMetadataEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received task metadata: %s \n", string(body))

	var taskMetadata TaskResponse
	if err = json.Unmarshal(body, &taskMetadata); err != nil {
		return fmt.Errorf("unable to parse response body: %v", err)
	}

	if err = verifyTaskMetadataResponse(body); err != nil {
		return err
	}

	return nil
}

func verifyContainerStats(client *http.Client, containerStatsEndpoint string) error {
	body, err := metadataResponse(client, containerStatsEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received container stats: %s \n", string(body))

	var containerStats types.StatsJSON
	err = json.Unmarshal(body, &containerStats)
	if err != nil {
		return fmt.Errorf("container stats: unable to parse response body: %v", err)
	}

	if isBridgeNetworkMode {
		// networks field should be populated in bridge mode
		if containerStats.Networks == nil {
			return errors.New("container stats: field networks should not be empty")
		}
	}

	return nil
}

func verifyTaskStats(client *http.Client, taskStatsEndpoint string) error {
	body, err := metadataResponse(client, taskStatsEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received task stats: %s \n", string(body))

	var taskStats map[string]*types.StatsJSON
	err = json.Unmarshal(body, &taskStats)
	if err != nil {
		return fmt.Errorf("task stats: unable to parse response body: %v", err)
	}

	if isBridgeNetworkMode {
		for container, containerStats := range taskStats {
			// networks field should be populated in bridge mode
			if containerStats.Networks == nil {
				return fmt.Errorf("task stats: field networks for container %s should not be empty", container)
			}
		}
	}

	return nil
}

func verifyTaskMetadataResponse(taskMetadataRawMsg json.RawMessage) error {
	var err error
	taskMetadataResponseMap := make(map[string]json.RawMessage)
	json.Unmarshal(taskMetadataRawMsg, &taskMetadataResponseMap)

	taskExpectedFieldEqualMap := map[string]interface{}{
		"DesiredStatus": "RUNNING",
		"KnownStatus":   "RUNNING",
	}

	taskExpectedFieldNotEmptyArray := []string{"Cluster", "TaskARN", "Family", "Revision", "PullStartedAt", "PullStoppedAt", "Containers", "AvailabilityZone"}
	if checkContainerInstanceTags {
		taskExpectedFieldNotEmptyArray = append(taskExpectedFieldNotEmptyArray, "ContainerInstanceTags")
	}

	for fieldName, fieldVal := range taskExpectedFieldEqualMap {
		if err = fieldEqual(taskMetadataResponseMap, fieldName, fieldVal); err != nil {
			return err
		}
	}

	for _, fieldName := range taskExpectedFieldNotEmptyArray {
		if err = fieldNotEmpty(taskMetadataResponseMap, fieldName); err != nil {
			return err
		}
	}

	var containersMetadataResponseArray []json.RawMessage
	json.Unmarshal(taskMetadataResponseMap["Containers"], &containersMetadataResponseArray)

	if isAWSVPCNetworkMode {
		if len(containersMetadataResponseArray) != 2 {
			return fmt.Errorf("incorrect number of containers, expected 2, received %d",
				len(containersMetadataResponseArray))
		}

		ok, err := isPauseContainer(containersMetadataResponseArray[0])
		if err != nil {
			return err
		}
		if ok {
			return verifyContainerMetadataResponse(containersMetadataResponseArray[1])
		} else {
			return verifyContainerMetadataResponse(containersMetadataResponseArray[0])
		}
	} else {
		if len(containersMetadataResponseArray) != 1 {
			return fmt.Errorf("incorrect number of containers, expected 1, received %d",
				len(containersMetadataResponseArray))
		}

		return verifyContainerMetadataResponse(containersMetadataResponseArray[0])
	}

	return nil
}

func isPauseContainer(containerMetadataRawMsg json.RawMessage) (bool, error) {
	var err error
	containerMetadataResponseMap := make(map[string]json.RawMessage)
	json.Unmarshal(containerMetadataRawMsg, &containerMetadataResponseMap)

	if err = fieldNotEmpty(containerMetadataResponseMap, "Name"); err != nil {
		return false, err
	}

	var actualContainerName string
	json.Unmarshal(containerMetadataResponseMap["Name"], &actualContainerName)

	if actualContainerName == "~internal~ecs~pause" {
		return true, nil
	}

	return false, nil
}

func verifyContainerMetadataResponse(containerMetadataRawMsg json.RawMessage) error {
	var err error
	containerMetadataResponseMap := make(map[string]json.RawMessage)
	json.Unmarshal(containerMetadataRawMsg, &containerMetadataResponseMap)

	containerExpectedFieldEqualMap := map[string]interface{}{
		"Name":          "v3-task-endpoint-validator",
		"Image":         "127.0.0.1:51670/amazon/amazon-ecs-v3-task-endpoint-validator:latest",
		"DesiredStatus": "RUNNING",
		"KnownStatus":   "RUNNING",
		"Type":          "NORMAL",
	}

	taskExpectedFieldNotEmptyArray := []string{"DockerId", "DockerName", "ImageID", "Limits", "CreatedAt", "StartedAt", "Health", "Networks"}

	for fieldName, fieldVal := range containerExpectedFieldEqualMap {
		if err = fieldEqual(containerMetadataResponseMap, fieldName, fieldVal); err != nil {
			return err
		}
	}

	for _, fieldName := range taskExpectedFieldNotEmptyArray {
		if err = fieldNotEmpty(containerMetadataResponseMap, fieldName); err != nil {
			return err
		}
	}

	if err = verifyLimitResponse(containerMetadataResponseMap["Limits"]); err != nil {
		return err
	}
	if err = verifyNetworksResponse(containerMetadataResponseMap["Networks"]); err != nil {
		return err
	}

	return nil
}

func verifyLimitResponse(limitRawMsg json.RawMessage) error {
	var err error
	limitResponseMap := make(map[string]json.RawMessage)
	json.Unmarshal(limitRawMsg, &limitResponseMap)

	limitExpectedFieldEqualMap := map[string]interface{}{
		"CPU":    float64(0),
		"Memory": float64(50),
	}

	for fieldName, fieldVal := range limitExpectedFieldEqualMap {
		if err = fieldEqual(limitResponseMap, fieldName, fieldVal); err != nil {
			return err
		}
	}

	return nil
}

func verifyNetworksResponse(networksRawMsg json.RawMessage) error {

	var err error

	var networksResponseArray []json.RawMessage
	json.Unmarshal(networksRawMsg, &networksResponseArray)

	if len(networksResponseArray) == 1 {
		networkResponseMap := make(map[string]json.RawMessage)
		json.Unmarshal(networksResponseArray[0], &networkResponseMap)

		var actualFieldVal interface{}
		json.Unmarshal(networkResponseMap["NetworkMode"], &actualFieldVal)

		if _, ok := networkModes[actualFieldVal.(string)]; !ok {
			return errors.Errorf("network mode is incorrect: %s", actualFieldVal)
		}
		if actualFieldVal != "host" {
			if err = fieldNotEmpty(networkResponseMap, "IPv4Addresses"); err != nil {
				return err
			}

			var ipv4AddressesResponseArray []json.RawMessage
			json.Unmarshal(networkResponseMap["IPv4Addresses"], &ipv4AddressesResponseArray)

			if len(ipv4AddressesResponseArray) != 1 {
				return fmt.Errorf("incorrect number of IPv4Addresses, expected 1, received %d",
					len(ipv4AddressesResponseArray))
			}
		}
		if actualFieldVal == "awsvpc" {
			isAWSVPCNetworkMode = true
		} else if actualFieldVal == "bridge" {
			isBridgeNetworkMode = true
		}
	} else {
		return fmt.Errorf("incorrect number of networks, expected 1, received %d",
			len(networksResponseArray))
	}

	return nil
}

func metadataResponse(client *http.Client, endpoint string) ([]byte, error) {
	var resp []byte
	var err error
	for i := 0; i < maxRetries; i++ {
		resp, err = metadataResponseOnce(client, endpoint)
		if err == nil {
			return resp, nil
		}
		fmt.Fprintf(os.Stderr, "Attempt [%d/%d]: unable to get metadata response from '%s': %v",
			i, maxRetries, endpoint, err)
		time.Sleep(durationBetweenRetries)
	}

	return nil, err
}

func metadataResponseOnce(client *http.Client, endpoint string) ([]byte, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to get response: %v", err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("incorrect status code  %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %v", err)
	}

	return body, nil
}

func notEmptyErrMsg(fieldName string) error {
	return fmt.Errorf("field %s should not be empty", fieldName)
}

func fieldNotEmpty(rawMsgMap map[string]json.RawMessage, fieldName string) error {
	if rawMsgMap[fieldName] == nil {
		return notEmptyErrMsg(fieldName)
	}

	return nil
}

func fieldEqual(rawMsgMap map[string]json.RawMessage, fieldName string, fieldVal interface{}) error {
	if err := fieldNotEmpty(rawMsgMap, fieldName); err != nil {
		return err
	}

	var actualFieldVal interface{}
	json.Unmarshal(rawMsgMap[fieldName], &actualFieldVal)

	if fieldVal != actualFieldVal {
		return fmt.Errorf("incorrect field value for field %s, expected %v, received %v",
			fieldName, fieldVal, actualFieldVal)
	}

	return nil
}

func main() {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	networkModes = map[string]bool{"awsvpc": true, "bridge": true, "host": true, "default": true}

	// If the image is built with option to check Tags
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) > 0 {
		if argsWithoutProg[0] == "CheckTags" {
			checkContainerInstanceTags = true
		}
	}

	// Wait for the Health information to be ready
	time.Sleep(5 * time.Second)

	isAWSVPCNetworkMode = false
	isBridgeNetworkMode = false
	v3BaseEndpoint := os.Getenv(containerMetadataEnvVar)
	containerMetadataPath := v3BaseEndpoint
	taskMetadataPath := v3BaseEndpoint
	if checkContainerInstanceTags {
		taskMetadataPath += "/taskWithTags"
	} else {
		taskMetadataPath += "/task"
	}
	containerStatsPath := v3BaseEndpoint + "/stats"
	taskStatsPath := v3BaseEndpoint + "/task/stats"

	if err := verifyContainerMetadata(client, containerMetadataPath); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get container metadata: %v\n", err)
		os.Exit(1)
	}

	if err := verifyTaskMetadata(client, taskMetadataPath); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get task metadata: %v\n", err)
		os.Exit(1)
	}

	if err := verifyContainerStats(client, containerStatsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get container stats: %v\n", err)
		os.Exit(1)
	}

	if err := verifyTaskStats(client, taskStatsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get task stats: %v\n", err)
		os.Exit(1)
	}

	os.Exit(42)
}
