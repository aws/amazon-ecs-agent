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
)

const (
	containerMetadataEnvVar      = "ECS_CONTAINER_METADATA_URI"
	eiDeviceListEndpointFormat   = "%s/associations/elastic-inference"
	eiDeviceDetailEndpointFormat = "%s/associations/elastic-inference/%s"

	maxRetries             = 4
	durationBetweenRetries = time.Second
)

var (
	// expected list of association names for the test container
	expectedAssociations               = []string{"device_1"}
	expectedAssociationsResponseFields = []string{"Associations"}
)

// AssociationsResponse defines the schema for the associations response JSON object
type AssociationsResponse struct {
	Associations []string `json:"Associations"`
}

// verify all the field names in the response are what we expected. this is needed because a direct unmarshal
// is not case sensitive so can't ensure that the response is in valid format
func verifyResponseFieldNames(expectedFields []string, responseBody []byte, responseName string) error {
	var responseMap map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &responseMap); err != nil {
		fmt.Errorf("unable to unmarshal %s response to response map", responseName)
	}

	for _, field := range expectedFields {
		if _, ok := responseMap[field]; !ok {
			return fmt.Errorf("missing field '%s' in %s response", field, responseName)
		}
	}

	return nil
}

func verifyAssociatedEIDevice(client *http.Client, containerMetadataBasePath, deviceName string) error {
	eiDeviceDetailEndpoint := fmt.Sprintf(eiDeviceDetailEndpointFormat, containerMetadataBasePath, deviceName)
	body, err := endpointResponse(client, eiDeviceDetailEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received ei association response: %s \n", string(body))

	return nil
}

func verifyAssociatedEIDevices(client *http.Client, containerMetadataBasePath string) error {
	eiDeviceListEndpoint := fmt.Sprintf(eiDeviceListEndpointFormat, containerMetadataBasePath)
	body, err := endpointResponse(client, eiDeviceListEndpoint)
	if err != nil {
		return err
	}

	fmt.Printf("Received ei associations response: %s \n", string(body))

	if err = verifyResponseFieldNames(expectedAssociationsResponseFields, body, "associations"); err != nil {
		return err
	}

	var associationsResponse AssociationsResponse
	if err = json.Unmarshal(body, &associationsResponse); err != nil {
		return fmt.Errorf("unable to parse response body: %v", err)
	}

	associations := associationsResponse.Associations
	if !matchWithoutOrder(associations, expectedAssociations) {
		return fmt.Errorf("associations incorrect: expected: %v, actual: %v", expectedAssociations,
			associations)
	}

	for _, associationName := range associations {
		if err = verifyAssociatedEIDevice(client, containerMetadataBasePath, associationName); err != nil {
			return err
		}
	}

	return nil
}

// helper func to check whether two lists of (distinct) strings matches without order
func matchWithoutOrder(a1, a2 []string) bool {
	if len(a1) != len(a2) {
		return false
	}

	m := make(map[string]bool)

	for _, x := range a1 {
		m[x] = true
	}

	for _, y := range a2 {
		if _, ok := m[y]; !ok {
			return false
		}
	}

	return true
}

func endpointResponse(client *http.Client, endpoint string) ([]byte, error) {
	var resp []byte
	var err error
	for i := 0; i < maxRetries; i++ {
		resp, err = endpointResponseOnce(client, endpoint)
		if err == nil {
			return resp, nil
		}
		fmt.Fprintf(os.Stderr, "Attempt [%d/%d]: unable to get response from '%s': %v",
			i, maxRetries, endpoint, err)
		time.Sleep(durationBetweenRetries)
	}

	return nil, err
}

func endpointResponseOnce(client *http.Client, endpoint string) ([]byte, error) {
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

func main() {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	containerMetadataBasePath := os.Getenv(containerMetadataEnvVar)
	if containerMetadataBasePath == "" {
		fmt.Printf("Container metadata url env %s is not set", containerMetadataEnvVar)
		os.Exit(1)
	}

	fmt.Printf("Container metadata url set to %s\n", containerMetadataBasePath)

	if err := verifyAssociatedEIDevices(client, containerMetadataBasePath); err != nil {
		fmt.Printf("Failed to verify associated ei devices: %v\n", err)
		os.Exit(1)
	}

	os.Exit(42)
}
