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

package asmsecret

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name               string
		InASMSecretStatus  ASMSecretStatus
		OutASMSecretStatus string
	}{
		{
			Name:               "ToStringASMSecretStatusNone",
			InASMSecretStatus:  ASMSecretStatusNone,
			OutASMSecretStatus: "NONE",
		},
		{
			Name:               "ToStringASMSecretCreated",
			InASMSecretStatus:  ASMSecretCreated,
			OutASMSecretStatus: "CREATED",
		},
		{
			Name:               "ToStringASMSecretRemoved",
			InASMSecretStatus:  ASMSecretRemoved,
			OutASMSecretStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutASMSecretStatus, c.InASMSecretStatus.String())
		})
	}
}

func TestMarshalNilASMSecretStatus(t *testing.T) {
	var status *ASMSecretStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalASMSecretStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InASMSecretStatus  ASMSecretStatus
		OutASMSecretStatus string
	}{
		{
			Name:               "MarshallASMSecretStatusNone",
			InASMSecretStatus:  ASMSecretStatusNone,
			OutASMSecretStatus: "\"NONE\"",
		},
		{
			Name:               "MarshallASMSecretCreated",
			InASMSecretStatus:  ASMSecretCreated,
			OutASMSecretStatus: "\"CREATED\"",
		},
		{
			Name:               "MarshallASMSecretRemoved",
			InASMSecretStatus:  ASMSecretRemoved,
			OutASMSecretStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InASMSecretStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutASMSecretStatus, string(bytes[:]))
		})
	}

}

func TestUnmarshalASMSecretStatus(t *testing.T) {
	cases := []struct {
		Name               string
		InASMSecretStatus  string
		OutASMSecretStatus ASMSecretStatus
		ShouldError        bool
	}{
		{
			Name:               "UnmarshallASMSecretStatusNone",
			InASMSecretStatus:  "\"NONE\"",
			OutASMSecretStatus: ASMSecretStatusNone,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallASMSecretCreated",
			InASMSecretStatus:  "\"CREATED\"",
			OutASMSecretStatus: ASMSecretCreated,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallASMSecretRemoved",
			InASMSecretStatus:  "\"REMOVED\"",
			OutASMSecretStatus: ASMSecretRemoved,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallASMSecretStatusNull",
			InASMSecretStatus:  "null",
			OutASMSecretStatus: ASMSecretStatusNone,
			ShouldError:        false,
		},
		{
			Name:               "UnmarshallASMSecretStatusNonString",
			InASMSecretStatus:  "1",
			OutASMSecretStatus: ASMSecretStatusNone,
			ShouldError:        true,
		},
		{
			Name:               "UnmarshallASMSecretStatusUnmappedStatus",
			InASMSecretStatus:  "\"LOL\"",
			OutASMSecretStatus: ASMSecretStatusNone,
			ShouldError:        true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			var status ASMSecretStatus
			err := json.Unmarshal([]byte(c.InASMSecretStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
				assert.Equal(t, c.OutASMSecretStatus, status)
			}
		})
	}
}
