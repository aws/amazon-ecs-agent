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

package credentialspec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name                    string
		InCredentialSpecStatus  CredentialSpecStatus
		OutCredentialSpecStatus string
	}{
		{
			Name:                    "ToStringCredentialSpecStatusNone",
			InCredentialSpecStatus:  CredentialSpecStatusNone,
			OutCredentialSpecStatus: "NONE",
		},
		{
			Name:                    "ToStringCredentialSpecCreated",
			InCredentialSpecStatus:  CredentialSpecCreated,
			OutCredentialSpecStatus: "CREATED",
		},
		{
			Name:                    "ToStringCredentialSpecRemoved",
			InCredentialSpecStatus:  CredentialSpecRemoved,
			OutCredentialSpecStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutCredentialSpecStatus, c.InCredentialSpecStatus.String())
		})
	}
}

func TestMarshalNilCredentialSpecStatus(t *testing.T) {
	var status *CredentialSpecStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalCredentialSpecStatus(t *testing.T) {
	cases := []struct {
		Name                    string
		InCredentialSpecStatus  CredentialSpecStatus
		OutCredentialSpecStatus string
	}{
		{
			Name:                    "ToStringCredentialSpecStatusNone",
			InCredentialSpecStatus:  CredentialSpecStatusNone,
			OutCredentialSpecStatus: "\"NONE\"",
		},
		{
			Name:                    "ToStringCredentialSpecCreated",
			InCredentialSpecStatus:  CredentialSpecCreated,
			OutCredentialSpecStatus: "\"CREATED\"",
		},
		{
			Name:                    "ToStringCredentialSpecRemoved",
			InCredentialSpecStatus:  CredentialSpecRemoved,
			OutCredentialSpecStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InCredentialSpecStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutCredentialSpecStatus, string(bytes[:]))
		})
	}

}

func TestUnmarshalSSMSecretStatus(t *testing.T) {
	cases := []struct {
		Name                    string
		InCredentialSpecStatus  string
		OutCredentialSpecStatus CredentialSpecStatus
		ShouldError             bool
	}{
		{
			Name:                    "UnmarshallCredentialSpecStatusNoneNone",
			InCredentialSpecStatus:  "\"NONE\"",
			OutCredentialSpecStatus: CredentialSpecStatusNone,
			ShouldError:             false,
		},
		{
			Name:                    "UnmarshallCredentialSpecCreated",
			InCredentialSpecStatus:  "\"CREATED\"",
			OutCredentialSpecStatus: CredentialSpecCreated,
			ShouldError:             false,
		},
		{
			Name:                    "UnmarshallCredentialSpecRemoved",
			InCredentialSpecStatus:  "\"REMOVED\"",
			OutCredentialSpecStatus: CredentialSpecRemoved,
			ShouldError:             false,
		},
		{
			Name:                    "UnmarshallCredentialSpecStatusNull",
			InCredentialSpecStatus:  "null",
			OutCredentialSpecStatus: CredentialSpecStatusNone,
			ShouldError:             false,
		},
		{
			Name:                    "UnmarshallCredentialSpecStatusNonString",
			InCredentialSpecStatus:  "1",
			OutCredentialSpecStatus: CredentialSpecStatusNone,
			ShouldError:             true,
		},
		{
			Name:                    "UnmarshallCredentialSpecUnmappedStatus",
			InCredentialSpecStatus:  "\"LOL\"",
			OutCredentialSpecStatus: CredentialSpecStatusNone,
			ShouldError:             true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			var status CredentialSpecStatus
			err := json.Unmarshal([]byte(c.InCredentialSpecStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
				assert.Equal(t, c.OutCredentialSpecStatus, status)
			}
		})
	}
}
