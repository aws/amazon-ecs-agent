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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/pkg/errors"
)

// Hex encoding SHA256 of an empty string
// Ref: https://github.com/aws/aws-sdk-go-v2/blob/ad4fc4c28b5e589872821d34c8e3a25cc6bfe6f3/aws/signer/v4/v4.go#L254-L260
const emptyBodySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// SignHTTPRequest signs an http.Request struct with authv4 using the given region, service, and credentials.
func SignHTTPRequest(req *http.Request, region, service string, creds *aws.CredentialsCache, body io.ReadSeeker) error {
	signer := v4.NewSigner()
	credsValue, err := creds.Retrieve(req.Context())
	if err != nil {
		logger.Warn(fmt.Sprintf("Retrieving credentials failed: %v", err))
		return errors.Wrap(err, "aws sdk http signer: failed to retrieve credentials")
	}

	payloadHash := emptyBodySHA256
	if body != nil {
		// Set the body reader to the beginning
		if _, err := body.Seek(0, io.SeekStart); err != nil {
			logger.Warn(fmt.Sprintf("Error resetting body reader: %v", err))
			// Continue with signing as we have the hash
		}

		hasher := sha256.New()
		if _, err := io.Copy(hasher, body); err != nil {
			logger.Warn(fmt.Sprintf("Error hashing request body: %v", err))
			return errors.Wrap(err, "aws sdk http signer: failed to hash request body")
		}

		// Reset the body reader to the beginning
		if _, err := body.Seek(0, io.SeekStart); err != nil {
			logger.Warn(fmt.Sprintf("Error resetting body reader: %v", err))
			// Continue with signing as we have the hash
		}
		payloadHash = hex.EncodeToString(hasher.Sum(nil))
	}

	err = signer.SignHTTP(req.Context(), credsValue, req, payloadHash, service, region, time.Now())
	if err != nil {
		logger.Warn(fmt.Sprintf("Signing HTTP request failed: %v", err))
		return errors.Wrap(err, "aws sdk http signer: failed to sign http request")
	}
	return nil
}
