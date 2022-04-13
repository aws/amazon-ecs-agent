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

package acsclient

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/stretchr/testify/require"
)

var acsErr *acsError

func init() {
	acsErr = &acsError{}
}

func TestInvalidInstanceException(t *testing.T) {
	errMsg := "Invalid instance"
	err := acsErr.NewError(&ecsacs.InvalidInstanceException{Message_: &errMsg})

	require.False(t, err.Retry(), "Expected InvalidInstanceException to not be retriable")
	require.EqualError(t, err, "InvalidInstanceException: "+errMsg)
}

func TestInvalidClusterException(t *testing.T) {
	errMsg := "Invalid cluster"
	err := acsErr.NewError(&ecsacs.InvalidClusterException{Message_: &errMsg})

	require.False(t, err.Retry(), "Expected to not be retriable")
	require.EqualError(t, err, "InvalidClusterException: "+errMsg)
}

func TestServerException(t *testing.T) {
	err := acsErr.NewError(&ecsacs.ServerException{Message_: nil})

	require.True(t, err.Retry(), "Server exceptions are retriable")
	require.EqualError(t, err, "ServerException: null")
}

func TestGenericErrorConversion(t *testing.T) {
	err := acsErr.NewError(errors.New("generic error"))

	require.True(t, err.Retry(), "Should default to retriable")
	require.EqualError(t, err, "ACSError: generic error")
}

func TestSomeRandomTypeConversion(t *testing.T) {
	// This is really just an 'it doesn't panic' check.
	err := acsErr.NewError(t)
	require.True(t, err.Retry(), "Should default to retriable")
	require.True(t, strings.HasPrefix(err.Error(), "ACSError: Unknown error"))
}

func TestBadlyTypedMessage(t *testing.T) {
	// Another 'does not panic' check
	err := acsErr.NewError(struct{ Message int }{1})
	require.True(t, err.Retry(), "Should default to retriable")
	require.NotNil(t, err.Error())
}
