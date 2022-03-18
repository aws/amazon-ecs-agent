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

package ssmsecret

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name               string
		InSSMSecretStatus  SSMSecretStatus
		OutSSMSecretStatus string
	}{
		{
			Name:               "ToStringSSMSecretStatusNone",
			InSSMSecretStatus:  SSMSecretStatusNone,
			OutSSMSecretStatus: "NONE",
		},
		{
			Name:               "ToStringSSMSecretCreated",
			InSSMSecretStatus:  SSMSecretCreated,
			OutSSMSecretStatus: "CREATED",
		},
		{
			Name:               "ToStringSSMSecretRemoved",
			InSSMSecretStatus:  SSMSecretRemoved,
			OutSSMSecretStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutSSMSecretStatus, c.InSSMSecretStatus.String())
		})
	}
}

func TestMarshalNilSSMSecretStatus(t *testing.T) {
	var status *SSMSecretStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalSSMSecretStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InSSMSecretStatus  SSMSecretStatus
		OutSSMSecretStatus string
	}{
		{
			Name:               "MarshallSSMSecretStatusNone",
			InSSMSecretStatus:  SSMSecretStatusNone,
			OutSSMSecretStatus: "\"NONE\"",
		},
		{
			Name:               "MarshallSSMSecretCreated",
			InSSMSecretStatus:  SSMSecretCreated,
			OutSSMSecretStatus: "\"CREATED\"",
		},
		{
			Name:               "MarshallSSMSecretRemoved",
			InSSMSecretStatus:  SSMSecretRemoved,
			OutSSMSecretStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InSSMSecretStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutSSMSecretStatus, string(bytes[:]))
		})
	}

}

func TestUnmarshalSSMSecretStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InSSMSecretStatus  string
		OutSSMSecretStatus SSMSecretStatus
		ShouldError        bool
	}{
		{
			Name:               "UnmarshallSSMSecretStatusNone",
			InSSMSecretStatus:  "\"NONE\"",
			OutSSMSecretStatus: SSMSecretStatusNone,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallSSMSecretCreated",
			InSSMSecretStatus:  "\"CREATED\"",
			OutSSMSecretStatus: SSMSecretCreated,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallSSMSecretRemoved",
			InSSMSecretStatus:  "\"REMOVED\"",
			OutSSMSecretStatus: SSMSecretRemoved,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallSSMSecretStatusNull",
			InSSMSecretStatus:  "null",
			OutSSMSecretStatus: SSMSecretStatusNone,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallSSMSecretStatusNonString",
			InSSMSecretStatus:  "1",
			OutSSMSecretStatus: SSMSecretStatusNone,
			ShouldError:        true,
		},
		{
			Name:               "UnmarshallSSMSecretStatusUnmappedStatus",
			InSSMSecretStatus:  "\"LOL\"",
			OutSSMSecretStatus: SSMSecretStatusNone,
			ShouldError:        true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			var status SSMSecretStatus
			err := json.Unmarshal([]byte(c.InSSMSecretStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
				assert.Equal(t, c.OutSSMSecretStatus, status)
			}
		})
	}
}
