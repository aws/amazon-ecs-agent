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

package authv4

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/sign"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/signable"
	"net/http"
)

type Signer interface {
	Sign(signable.Signable) error
}

// Implements Signer
type DefaultSigner struct {
	credentials.AWSCredentialProvider

	Region, Service string

	ExtraHeaders []string

	Signer *sign.Signer
}

// Signs an http.Request by mutating it
type HttpSigner interface {
	SignHttpRequest(*http.Request) error
}

// A signed round-tripper signs a request and subsequently returns a response.
type RoundTripperSigner http.RoundTripper

type DefaultRoundTripSigner struct {
	HttpSigner

	Transport http.RoundTripper
}
