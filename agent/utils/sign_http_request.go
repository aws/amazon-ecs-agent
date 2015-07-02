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

package utils

import (
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/internal/signer/v4"
)

// SignHTTPRequest signs an http.Request struct with authv4 using the given region, service, and credentials.
func SignHTTPRequest(req *http.Request, region, service string, creds *credentials.Credentials) {
	v4.Sign(&aws.Request{
		Service: &aws.Service{
			SigningRegion: region,
			SigningName:   service,
			Config: &aws.Config{
				Credentials: creds,
			},
		},
		HTTPRequest: req,
		Time:        time.Now(),
	})
}
