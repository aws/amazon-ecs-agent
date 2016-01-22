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

package ecr

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
)

const opGetAuthorizationToken = "GetAuthorizationToken"

// GetAuthorizationTokenRequest generates a request for the GetAuthorizationToken operation.
func (c *ECR) GetAuthorizationTokenRequest(input *GetAuthorizationTokenInput) (req *request.Request, output *GetAuthorizationTokenOutput) {
	op := &request.Operation{
		Name:       opGetAuthorizationToken,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &GetAuthorizationTokenInput{}
	}

	req = c.newRequest(op, input, output)
	output = &GetAuthorizationTokenOutput{}
	req.Data = output
	return
}

func (c *ECR) GetAuthorizationToken(input *GetAuthorizationTokenInput) (*GetAuthorizationTokenOutput, error) {
	req, out := c.GetAuthorizationTokenRequest(input)
	err := req.Send()
	return out, err
}

type AuthorizationData struct {
	_ struct{} `type:"structure"`

	AuthorizationToken *string `locationName:"authorizationToken" type:"string"`

	ExpiresAt *time.Time `locationName:"expiresAt" type:"timestamp" timestampFormat:"unix"`

	ProxyEndpoint *string `locationName:"proxyEndpoint" type:"string"`
}

// String returns the string representation
func (s AuthorizationData) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AuthorizationData) GoString() string {
	return s.String()
}

type GetAuthorizationTokenInput struct {
	_ struct{} `type:"structure"`

	RegistryIds []*string `locationName:"registryIds" type:"list"`
}

// String returns the string representation
func (s GetAuthorizationTokenInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetAuthorizationTokenInput) GoString() string {
	return s.String()
}

type GetAuthorizationTokenOutput struct {
	_ struct{} `type:"structure"`

	AuthorizationData []*AuthorizationData `locationName:"authorizationData" type:"list"`
}

// String returns the string representation
func (s GetAuthorizationTokenOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetAuthorizationTokenOutput) GoString() string {
	return s.String()
}
