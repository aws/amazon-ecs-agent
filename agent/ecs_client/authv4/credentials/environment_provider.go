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

package credentials

import (
	"errors"
	"os"
)

type EnvironmentCredentialProvider struct{}

func NewEnvironmentCredentialProvider() *EnvironmentCredentialProvider {
	return new(EnvironmentCredentialProvider)
}

// Credentials pulls the credentials from common environment variables.
func (ecp EnvironmentCredentialProvider) Credentials() (*AWSCredentials, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if len(accessKey) > 0 && len(secretKey) > 0 {
		if len(sessionToken) > 0 {
			return &AWSCredentials{accessKey, secretKey, sessionToken}, nil
		}
		return &AWSCredentials{AccessKey: accessKey, SecretKey: secretKey}, nil
	}
	return nil, errors.New("Unable to find credentials in the environment")
}
