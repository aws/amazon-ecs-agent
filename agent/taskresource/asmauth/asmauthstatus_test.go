// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package asmauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name             string
		InASMAuthStatus  ASMAuthStatus
		OutASMAuthStatus string
	}{
		{
			Name:             "ToStringASMAuthStatusNone",
			InASMAuthStatus:  ASMAuthStatusNone,
			OutASMAuthStatus: "NONE",
		},
		{
			Name:             "ToStringASMAuthStatusCreated",
			InASMAuthStatus:  ASMAuthStatusCreated,
			OutASMAuthStatus: "CREATED",
		},
		{
			Name:             "ToStringASMAuthStatusRemoved",
			InASMAuthStatus:  ASMAuthStatusRemoved,
			OutASMAuthStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutASMAuthStatus, c.InASMAuthStatus.String())
		})
	}
}

func TestMarshalNilASMAuthStatus(t *testing.T) {
	var status *ASMAuthStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalASMAuthStatus(t *testing.T) {
	cases := []struct {
		Name             string
		InASMAuthStatus  ASMAuthStatus
		OutASMAuthStatus string
	}{
		{
			Name:             "MarshallASMAuthStatusNone",
			InASMAuthStatus:  ASMAuthStatusNone,
			OutASMAuthStatus: "\"NONE\"",
		},
		{
			Name:             "MarshallASMAuthStatusCreated",
			InASMAuthStatus:  ASMAuthStatusCreated,
			OutASMAuthStatus: "\"CREATED\"",
		},
		{
			Name:             "MarshallASMAuthStatusRemoved",
			InASMAuthStatus:  ASMAuthStatusRemoved,
			OutASMAuthStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InASMAuthStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutASMAuthStatus, string(bytes[:]))
		})
	}

}

func TestUnmarshalASMAuthStatus(t *testing.T) {
	cases := []struct {
		Name             string
		InASMAuthStatus  string
		OutASMAuthStatus ASMAuthStatus
		ShouldError      bool
	}{
		{
			Name:             "UnmarshallASMAuthStatusNone",
			InASMAuthStatus:  "\"NONE\"",
			OutASMAuthStatus: ASMAuthStatusNone,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallASMAuthStatusCreated",
			InASMAuthStatus:  "\"CREATED\"",
			OutASMAuthStatus: ASMAuthStatusCreated,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallASMAuthStatusRemoved",
			InASMAuthStatus:  "\"REMOVED\"",
			OutASMAuthStatus: ASMAuthStatusRemoved,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallASMAuthStatusNull",
			InASMAuthStatus:  "null",
			OutASMAuthStatus: ASMAuthStatusNone,
			ShouldError:      false,
		},
		{
			Name:             "UnmarshallASMAuthStatusNonString",
			InASMAuthStatus:  "1",
			OutASMAuthStatus: ASMAuthStatusNone,
			ShouldError:      true,
		},
		{
			Name:             "UnmarshallASMAuthStatusUnmappedStatus",
			InASMAuthStatus:  "\"LOL\"",
			OutASMAuthStatus: ASMAuthStatusNone,
			ShouldError:      true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			var status ASMAuthStatus
			err := json.Unmarshal([]byte(c.InASMAuthStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
				assert.Equal(t, c.OutASMAuthStatus, status)
			}
		})
	}
}
