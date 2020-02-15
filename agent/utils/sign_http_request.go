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

package utils

import (
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// SignHTTPRequest signs an http.Request struct with authv4 using the given region, service, and credentials.
func SignHTTPRequest(req *http.Request, region, service string, creds *credentials.Credentials, body io.ReadSeeker) error {
	signer := v4.NewSigner(creds)
	_, err := signer.Sign(req, body, service, region, time.Now())
	if err != nil {
		seelog.Warnf("Signing HTTP request failed: %v", err)
		return errors.Wrap(err, "aws sdk http signer: failed to sign http request")
	}
	return nil
}
