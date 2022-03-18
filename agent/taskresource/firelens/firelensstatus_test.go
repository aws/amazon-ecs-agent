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

package firelens

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name              string
		InFirelensStatus  FirelensStatus
		OutFirelensStatus string
	}{
		{
			Name:              "ToStringFirelensStatusNone",
			InFirelensStatus:  FirelensStatusNone,
			OutFirelensStatus: "NONE",
		},
		{
			Name:              "ToStringFirelensStatusCreated",
			InFirelensStatus:  FirelensCreated,
			OutFirelensStatus: "CREATED",
		},
		{
			Name:              "ToStringFirelensStatusRemoved",
			InFirelensStatus:  FirelensRemoved,
			OutFirelensStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutFirelensStatus, c.InFirelensStatus.String())
		})
	}
}

func TestMarshalNilFirelensStatus(t *testing.T) {
	var status *FirelensStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalFirelensStatus(t *testing.T) {
	cases := []struct {
		Name              string
		InFirelensStatus  FirelensStatus
		OutFirelensStatus string
	}{
		{
			Name:              "MarshallFirelensStatusNone",
			InFirelensStatus:  FirelensStatusNone,
			OutFirelensStatus: "\"NONE\"",
		},
		{
			Name:              "MarshallFirelensCreated",
			InFirelensStatus:  FirelensCreated,
			OutFirelensStatus: "\"CREATED\"",
		},
		{
			Name:              "MarshallFirelensRemoved",
			InFirelensStatus:  FirelensRemoved,
			OutFirelensStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InFirelensStatus.MarshalJSON()
			assert.NoError(t, err)
			assert.Equal(t, c.OutFirelensStatus, string(bytes[:]))
		})
	}
}

func TestUnmarshalFirelensStatus(t *testing.T) {
	cases := []struct {
		Name              string
		InFirelensStatus  string
		OutFirelensStatus FirelensStatus
		ShouldError       bool
	}{
		{
			Name:              "UnmarshalFirelensStatusNone",
			InFirelensStatus:  "\"NONE\"",
			OutFirelensStatus: FirelensStatusNone,
			ShouldError:       false,
		},
		{
			Name:              "UnmarshalFirelensCreated",
			InFirelensStatus:  "\"CREATED\"",
			OutFirelensStatus: FirelensCreated,
			ShouldError:       false,
		},
		{
			Name:              "UnmarshalFirelensRemoved",
			InFirelensStatus:  "\"REMOVED\"",
			OutFirelensStatus: FirelensRemoved,
			ShouldError:       false,
		},
		{
			Name:              "UnmarshalFirelensStatusNull",
			InFirelensStatus:  "null",
			OutFirelensStatus: FirelensStatusNone,
			ShouldError:       false,
		},
		{
			Name:             "UnmarshalFirelensStatusNonString",
			InFirelensStatus: "1",
			ShouldError:      true,
		},
		{
			Name:             "UnmarshalFirelensStatusUnmappedStatus",
			InFirelensStatus: "\"INVALID\"",
			ShouldError:      true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var status FirelensStatus
			err := json.Unmarshal([]byte(c.InFirelensStatus), &status)
			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.OutFirelensStatus, status)
			}
		})
	}
}
