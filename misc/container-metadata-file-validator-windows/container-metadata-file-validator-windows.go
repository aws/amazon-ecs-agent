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

// This file is derived from misc/v3-task-endpoint-validator

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

const (
	containerMetadataFileEnvVar = "ECS_CONTAINER_METADATA_FILE"
	hasPublicIpEnvVar           = "HAS_PUBLIC_IP"
	MetadataInitialText         = "INITIAL"
	MetadataReadyText           = "READY"
)

// MetadataStatus specifies the current update status of the metadata file.
type MetadataStatus int32

// hasPublicIp indicates whether the test is run on an instance that has public ip
var hasPublicIp bool

// Represents the int32 representations for MetadataStatus
const (
	MetadataInitial MetadataStatus = iota
	MetadataReady
)

type metadataSerializer struct {
	Cluster                string         `json:"Cluster,omitempty"`
	ContainerInstanceARN   string         `json:"ContainerInstanceARN,omitempty"`
	TaskARN                string         `json:"TaskARN,omitempty"`
	TaskDefinitionFamily   string         `json:"TaskDefinitionFamily,omitempty"`
	TaskDefinitionRevision string         `json:"TaskDefinitionRevision,omitempty"`
	ContainerID            string         `json:"ContainerID,omitempty"`
	ContainerName          string         `json:"ContainerName,omitempty"`
	DockerContainerName    string         `json:"DockerContainerName,omitempty"`
	ImageID                string         `json:"ImageID,omitempty"`
	ImageName              string         `json:"ImageName,omitempty"`
	Ports                  []PortBinding  `json:"PortMappings,omitempty"`
	Networks               []Network      `json:"Networks,omitempty"`
	MetadataFileStatus     MetadataStatus `json:"MetadataFileStatus,omitempty"`
	AvailabilityZone       string         `json:"AvailabilityZone,omitempty"`
	HostPrivateIPv4Address string         `json:"HostPrivateIPv4Address,omitempty"`
	HostPublicIPv4Address  string         `json:"HostPublicIPv4Address,omitempty"`
}

type NetworkMetadata struct {
	networks []Network
}

// Network is a struct that keeps track of metadata of a network interface
type Network struct {
	NetworkMode   string   `json:"NetworkMode,omitempty"`
	IPv4Addresses []string `json:"IPv4Addresses,omitempty"`
	IPv6Addresses []string `json:"IPv6Addresses,omitempty"`
}

type PortBinding struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
	BindIP        string `json:"BindIp,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
}

// PortResponse defines the schema for portmapping response JSON
// object.
type PortResponse struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
}

func verifyContainerMetadataResponse(containerMetadataResponseMap map[string]json.RawMessage) error {
	var err error

	// Fields with known values
	containerExpectedFieldEqualMap := map[string]interface{}{
		"ContainerName":      "container-metadata-file-validator",
		"ImageName":          "amazon/amazon-ecs-container-metadata-file-validator-windows",
		"MetadataFileStatus": MetadataReadyText,
	}
	// Fields that change dynamically, not predictable
	taskExpectedFieldNotEmptyArray := []string{"TaskDefinitionFamily", "Cluster", "ContainerInstanceARN", "TaskARN", "TaskDefinitionRevision", "ContainerID", "DockerContainerName", "ImageID", "HostPrivateIPv4Address"}

	if hasPublicIp {
		taskExpectedFieldNotEmptyArray = append(taskExpectedFieldNotEmptyArray, "HostPublicIPv4Address")
	}

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

	return nil
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

// Allows unmarshaling from byte to MetadataStatus text form
func (status *MetadataStatus) UnmarshalText(text []byte) error {
	t := string(text)
	switch t {
	case MetadataInitialText:
		*status = MetadataInitial
	case MetadataReadyText:
		*status = MetadataReady
	default:
		return fmt.Errorf("failed unmarshalling MetadataStatus %s", text)
	}
	return nil
}

func main() {
	// Let metadata service start up and initialize with data
	time.Sleep(10 * time.Second)

	containerMetadataFile, err := os.Open(os.Getenv(containerMetadataFileEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to read container metadata file: %v\n", err)
		os.Exit(1)
	}

	metadataBytes, err := ioutil.ReadAll(containerMetadataFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to convert container metadata file to bytes: %v\n", err)
		os.Exit(1)
	}

	boolVar, err := strconv.ParseBool(os.Getenv(hasPublicIpEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse environment variable %s: %v", hasPublicIpEnvVar, err)
		os.Exit(1)
	}
	hasPublicIp = boolVar

	// Parse file into struct to print to awslogs
	var metadataResponse metadataSerializer
	json.Unmarshal(metadataBytes, &metadataResponse)
	fmt.Printf("Read container metadata file: %+v \n", metadataResponse)

	containerMetadataResponseMap := make(map[string]json.RawMessage)
	json.Unmarshal(metadataBytes, &containerMetadataResponseMap)

	if err = verifyContainerMetadataResponse(containerMetadataResponseMap); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get container metadata: %v\n", err)
		os.Exit(1)
	}

	os.Exit(42)
}
