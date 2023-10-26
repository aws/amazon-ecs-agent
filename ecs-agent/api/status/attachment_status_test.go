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

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusToString(t *testing.T) {
	testCases := []struct {
		status AttachmentStatus
		str    string
	}{
		{
			status: AttachmentNone,
			str:    "NONE",
		},
		{
			status: AttachmentAttached,
			str:    "ATTACHED",
		},
		{
			status: AttachmentDetached,
			str:    "DETACHED",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.str, func(t *testing.T) {
			assert.Equal(t, tc.str, (&tc.status).String())
		})
	}
}

func TestShouldSend(t *testing.T) {
	testCases := []struct {
		name       string
		status     AttachmentStatus
		shouldSend bool
	}{
		{
			name:       "NONE",
			status:     AttachmentNone,
			shouldSend: false,
		},
		{
			name:       "ATTACHED",
			status:     AttachmentAttached,
			shouldSend: true,
		},
		{
			name:       "DETACHED",
			status:     AttachmentDetached,
			shouldSend: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.shouldSend, (&tc.status).ShouldSend())
		})
	}
}
