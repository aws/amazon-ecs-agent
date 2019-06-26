// +build unit
// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package logrouter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name               string
		InLogRouterStatus  LogRouterStatus
		OutLogRouterStatus string
	}{
		{
			Name:               "ToStringLogRouterStatusNone",
			InLogRouterStatus:  LogRouterStatusNone,
			OutLogRouterStatus: "NONE",
		},
		{
			Name:               "ToStringLogRouterStatusCreated",
			InLogRouterStatus:  LogRouterCreated,
			OutLogRouterStatus: "CREATED",
		},
		{
			Name:               "ToStringLogRouterStatusRemoved",
			InLogRouterStatus:  LogRouterRemoved,
			OutLogRouterStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutLogRouterStatus, c.InLogRouterStatus.String())
		})
	}
}

func TestMarshalNilLogRouterStatus(t *testing.T) {
	var status *LogRouterStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalLogRouterStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InLogRouterStatus  LogRouterStatus
		OutLogRouterStatus string
	}{
		{
			Name:               "MarshallLogRouterStatusNone",
			InLogRouterStatus:  LogRouterStatusNone,
			OutLogRouterStatus: "\"NONE\"",
		},
		{
			Name:               "MarshallLogRouterCreated",
			InLogRouterStatus:  LogRouterCreated,
			OutLogRouterStatus: "\"CREATED\"",
		},
		{
			Name:               "MarshallLogRouterRemoved",
			InLogRouterStatus:  LogRouterRemoved,
			OutLogRouterStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InLogRouterStatus.MarshalJSON()
			assert.NoError(t, err)
			assert.Equal(t, c.OutLogRouterStatus, string(bytes[:]))
		})
	}
}

func TestUnmarshalLogRouterStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InLogRouterStatus  string
		OutLogRouterStatus LogRouterStatus
		ShouldError        bool
	}{
		{
			Name:               "UnmarshalLogRouterStatusNone",
			InLogRouterStatus:  "\"NONE\"",
			OutLogRouterStatus: LogRouterStatusNone,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshalLogRouterCreated",
			InLogRouterStatus:  "\"CREATED\"",
			OutLogRouterStatus: LogRouterCreated,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshalLogRouterRemoved",
			InLogRouterStatus:  "\"REMOVED\"",
			OutLogRouterStatus: LogRouterRemoved,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshalLogRouterStatusNull",
			InLogRouterStatus:  "null",
			OutLogRouterStatus: LogRouterStatusNone,
			ShouldError:        false,
		},
		{
			Name:              "UnmarshalLogRouterStatusNonString",
			InLogRouterStatus: "1",
			ShouldError:       true,
		},
		{
			Name:              "UnmarshalLogRouterStatusUnmappedStatus",
			InLogRouterStatus: "\"INVALID\"",
			ShouldError:       true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var status LogRouterStatus
			err := json.Unmarshal([]byte(c.InLogRouterStatus), &status)
			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.OutLogRouterStatus, status)
			}
		})
	}
}
