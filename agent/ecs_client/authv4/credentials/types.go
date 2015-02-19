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

import "errors"

type AWSCredentials struct {
	AccessKey string
	SecretKey string
	Token     string
}

func (creds *AWSCredentials) Credentials() (*AWSCredentials, error) {
	if creds == nil {
		return nil, errors.New("Credentials not set")
	}
	return creds, nil
}

type AWSCredentialProvider interface {
	Credentials() (*AWSCredentials, error)
}

type RefreshableAWSCredentialProvider interface {
	AWSCredentialProvider

	Refresh()

	NeedsRefresh() bool
}

type NoCredentialProviderError struct{}

func (ncp NoCredentialProviderError) Error() string {
	return "No credential provider was set"
}
