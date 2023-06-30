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
	"errors"
	"flag"
	"os"
)

const emptyCSIEndpoint = ""

type ServerOptions struct {
	// Endpoint is the endpoint that the driver server should listen on.
	Endpoint string
}

func GetServerOptions(fs *flag.FlagSet) (*ServerOptions, error) {
	serverOptions := &ServerOptions{}
	fs.StringVar(&serverOptions.Endpoint, "endpoint", emptyCSIEndpoint, "Endpoint for the CSI driver server")

	args := os.Args[1:]
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, errors.New("no argument is provided")
	}

	if serverOptions.Endpoint == emptyCSIEndpoint {
		return nil, errors.New("no endpoint is provided")
	}

	return serverOptions, nil
}
