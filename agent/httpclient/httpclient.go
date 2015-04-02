// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package httpclient provides a thin, but testable, wrapper around http.Client.
// It adds an ECS header to requests and provides an interface
package httpclient

import (
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/version"
)

const defaultTimeout = 10 * time.Minute

//go:generate mockgen.sh net/http RoundTripper mock/$GOFILE

// Default is the client used by this package; it should be overridden as
// desired for testing
var Default *http.Client = &http.Client{}

// Transport is the transport requests will be made over
var Transport = http.DefaultTransport

type ecsRoundTripper struct{}

func init() {
	Default.Transport = &ecsRoundTripper{}
	Default.Timeout = defaultTimeout
}

func userAgent() string {
	return version.String() + " (+http://aws.amazon.com/ecs/)"
}

func (*ecsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent())
	return Transport.RoundTrip(req)
}

func (*ecsRoundTripper) CancelRequest(req *http.Request) {
	if def, ok := http.DefaultTransport.(*http.Transport); ok {
		def.CancelRequest(req)
	}
}
