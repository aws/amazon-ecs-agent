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

package main

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetServerOptions(t *testing.T) {
	testFunc := func(t *testing.T, additionalArgs []string) (*ServerOptions, error) {
		flagSet := flag.NewFlagSet("test-flagset", flag.ContinueOnError)

		args := append([]string{
			"/bin/csi-driver",
		}, additionalArgs...)
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = args

		options, err := GetServerOptions(flagSet)
		return options, err
	}

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Only endpoint is given",
			testFunc: func(t *testing.T) {
				opts, err := testFunc(t, []string{"--endpoint=foo"})
				assert.NoError(t, err)
				assert.Equal(t, "foo", opts.Endpoint)
			},
		},
		{
			name: "No argument is given",
			testFunc: func(t *testing.T) {
				_, err := testFunc(t, nil)
				assert.EqualError(t, err, "no argument is provided")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
