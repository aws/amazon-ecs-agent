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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGPUAssociationMarshal(t *testing.T) {
	expectedAssociationMap := map[string]interface{}{
		"containers": []interface{}{"container1"},
		"content": map[string]interface{}{
			"encoding": "base64",
			"value":    "val",
		},
		"name": "gpu1",
		"type": "gpu",
	}

	association := Association{
		Containers: []string{"container1"},
		Content: EncodedString{
			Encoding: "base64",
			Value:    "val",
		},
		Name: "gpu1",
		Type: "gpu",
	}

	associationJSON, err := json.Marshal(association)
	assert.NoError(t, err)

	associationMap := make(map[string]interface{})
	json.Unmarshal(associationJSON, &associationMap)
	assert.Equal(t, expectedAssociationMap, associationMap)
}

func TestEIAssociationMarshal(t *testing.T) {
	expectedAssociationMap := map[string]interface{}{
		"containers": []interface{}{"container1"},
		"content": map[string]interface{}{
			"encoding": "base64",
			"value":    "val",
		},
		"name": "dev1",
		"type": "elastic-inference",
	}

	association := Association{
		Containers: []string{"container1"},
		Content: EncodedString{
			Encoding: "base64",
			Value:    "val",
		},
		Name: "dev1",
		Type: "elastic-inference",
	}

	associationJSON, err := json.Marshal(association)
	assert.NoError(t, err)

	associationMap := make(map[string]interface{})
	json.Unmarshal(associationJSON, &associationMap)
	assert.Equal(t, expectedAssociationMap, associationMap)
}

func TestEncodedStringMarshal(t *testing.T) {
	expectedEncodedStringMap := map[string]interface{}{
		"encoding": "base64",
		"value":    "val",
	}

	encodedString := EncodedString{
		Encoding: "base64",
		Value:    "val",
	}

	encodedStringJSON, err := json.Marshal(encodedString)
	assert.NoError(t, err)

	encodedStringMap := make(map[string]interface{})
	json.Unmarshal(encodedStringJSON, &encodedStringMap)
	assert.Equal(t, expectedEncodedStringMap, encodedStringMap)
}
