// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	// If enabled, the program performs a health check on an existing server
	// instead of starting a new server.
	HealthCheck bool
}

func GetServerOptions(fs *flag.FlagSet) (*ServerOptions, error) {
	serverOptions := &ServerOptions{}
	fs.StringVar(&serverOptions.Endpoint, "endpoint", emptyCSIEndpoint, "Endpoint for the CSI driver server")
	fs.BoolVar(&serverOptions.HealthCheck, "healthcheck", false, "Check health of an existing server")

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
