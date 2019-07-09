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

package efs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	cases := []struct {
		Name         string
		InEFSStatus  EFSStatus
		OutEFSStatus string
	}{
		{
			Name:         "ToStringEFSStatusNone",
			InEFSStatus:  EFSStatusNone,
			OutEFSStatus: "NONE",
		},
		{
			Name:         "ToStringEFSCreated",
			InEFSStatus:  EFSCreated,
			OutEFSStatus: "CREATED",
		},
		{
			Name:         "ToStringEFSRemoved",
			InEFSStatus:  EFSRemoved,
			OutEFSStatus: "REMOVED",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.OutEFSStatus, c.InEFSStatus.String())
		})
	}
}

func TestMarshalNilEFSStatus(t *testing.T) {
	var status *EFSStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Error(t, err)
}

func TestMarshalEFSStatus(t *testing.T) {
	cases := []struct {
		Name         string
		InEFSStatus  EFSStatus
		OutEFSStatus string
	}{
		{
			Name:         "MarshallEFSStatusNone",
			InEFSStatus:  EFSStatusNone,
			OutEFSStatus: "\"NONE\"",
		},
		{
			Name:         "MarshallEFSCreated",
			InEFSStatus:  EFSCreated,
			OutEFSStatus: "\"CREATED\"",
		},
		{
			Name:         "MarshallEFSRemoved",
			InEFSStatus:  EFSRemoved,
			OutEFSStatus: "\"REMOVED\"",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bytes, err := c.InEFSStatus.MarshalJSON()

			assert.NoError(t, err)
			assert.Equal(t, c.OutEFSStatus, string(bytes[:]))
		})
	}

}

func TestUnmarshalEFSStatus(t *testing.T) {
	cases := []struct {
		Name         string
		InEFSStatus  string
		OutEFSStatus EFSStatus
		ShouldError  bool
	}{
		{
			Name:         "UnmarshallEFSStatusNone",
			InEFSStatus:  "\"NONE\"",
			OutEFSStatus: EFSStatusNone,
			ShouldError:  false,
		},
		{
			Name:         "UnmarshallEFSCreated",
			InEFSStatus:  "\"CREATED\"",
			OutEFSStatus: EFSCreated,
			ShouldError:  false,
		},
		{
			Name:         "UnmarshallEFSRemoved",
			InEFSStatus:  "\"REMOVED\"",
			OutEFSStatus: EFSRemoved,
			ShouldError:  false,
		},
		{
			Name:         "UnmarshallEFSStatusNull",
			InEFSStatus:  "null",
			OutEFSStatus: EFSStatusNone,
			ShouldError:  false,
		},
		{
			Name:         "UnmarshallEFSStatusNonString",
			InEFSStatus:  "1",
			OutEFSStatus: EFSStatusNone,
			ShouldError:  true,
		},
		{
			Name:         "UnmarshallEFSStatusUnmappedStatus",
			InEFSStatus:  "\"LOL\"",
			OutEFSStatus: EFSStatusNone,
			ShouldError:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			var status EFSStatus
			err := json.Unmarshal([]byte(c.InEFSStatus), &status)

			if c.ShouldError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
				assert.Equal(t, c.OutEFSStatus, status)
			}
		})
	}
}
