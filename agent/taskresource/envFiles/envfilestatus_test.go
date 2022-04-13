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

package envFiles

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name             string
		InEnvFileStatus  EnvironmentFileStatus
		OutEnvFileStatus string
	}{
		{
			Name:             "ToStringEnvFileStatusNone",
			InEnvFileStatus:  EnvFileStatusNone,
			OutEnvFileStatus: "NONE",
		},
		{
			Name:             "ToStringEnvFileCreated",
			InEnvFileStatus:  EnvFileCreated,
			OutEnvFileStatus: "CREATED",
		},
		{
			Name:             "ToStringEnvFileRemoved",
			InEnvFileStatus:  EnvFileRemoved,
			OutEnvFileStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutEnvFileStatus, c.InEnvFileStatus.String())
		})
	}

}

func TestMarshalNilEnvFileStatus(t *testing.T) {
	var status *EnvironmentFileStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalEnvFileStatus(t *testing.T) {
	cases := []struct {
		Name             string
		InEnvFileStatus  EnvironmentFileStatus
		OutEnvFileStatus string
	}{
		{
			Name:             "MarshallEnvFileStatusNone",
			InEnvFileStatus:  EnvFileStatusNone,
			OutEnvFileStatus: "\"NONE\"",
		},
		{
			Name:             "MarshallEnvFileCreated",
			InEnvFileStatus:  EnvFileCreated,
			OutEnvFileStatus: "\"CREATED\"",
		},
		{
			Name:             "MarshallEnvFileRemoved",
			InEnvFileStatus:  EnvFileRemoved,
			OutEnvFileStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InEnvFileStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutEnvFileStatus, string(bytes[:]))
		})
	}
}

func TestUnmarshalEnvFileStatus(t *testing.T) {
	cases := []struct {
		Name             string
		InEnvFileStatus  string
		OutEnvFileStatus EnvironmentFileStatus
		ShouldError      bool
	}{
		{
			Name:             "UnmarshallEnvFileStatusNone",
			InEnvFileStatus:  "\"NONE\"",
			OutEnvFileStatus: EnvFileStatusNone,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallEnvFileCreated",
			InEnvFileStatus:  "\"CREATED\"",
			OutEnvFileStatus: EnvFileCreated,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallEnvFileRemoved",
			InEnvFileStatus:  "\"REMOVED\"",
			OutEnvFileStatus: EnvFileRemoved,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallEnvFileStatusNull",
			InEnvFileStatus:  "null",
			OutEnvFileStatus: EnvFileStatusNone,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallEnvFileStatusNonString",
			InEnvFileStatus:  "123",
			OutEnvFileStatus: EnvFileStatusNone,
			ShouldError:      true,
		},
		{
			Name:             "UnmarshallEnvFileStatusUnmappedStatus",
			InEnvFileStatus:  "\"WEEWOO\"",
			OutEnvFileStatus: EnvFileStatusNone,
			ShouldError:      true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var status EnvironmentFileStatus
			err := json.Unmarshal([]byte(c.InEnvFileStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.OutEnvFileStatus, status)
			}
		})
	}
}
